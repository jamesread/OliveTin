package executor

import (
	acl "github.com/OliveTin/OliveTin/internal/acl"
	config "github.com/OliveTin/OliveTin/internal/config"
	sv "github.com/OliveTin/OliveTin/internal/stringvariables"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"gopkg.in/yaml.v3"

	"bytes"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"
	"context"
)

var (
	metricActionsRequested = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "olivetin_actions_requested_count",
		Help: "The actions requested count",
	})
)

type ActionBinding struct {
	Action       *config.Action
	EntityPrefix string
	ConfigOrder  int
}

// Executor represents a helper class for executing commands. It's main method
// is ExecRequest
type Executor struct {
	Logs           map[string]*InternalLogEntry
	LogsByActionId map[string][]*InternalLogEntry

	MapActionIdToBinding     map[string]*ActionBinding
	MapActionIdToBindingLock sync.RWMutex

	Cfg *config.Config

	listeners []listener

	chainOfCommand []executorStepFunc
}

// ExecutionRequest is a request to execute an action. It's passed to an
// Executor. They're created from the grpcapi.
type ExecutionRequest struct {
	ActionTitle       string
	Action            *config.Action
	Arguments         map[string]string
	TrackingID        string
	Tags              []string
	Cfg               *config.Config
	AuthenticatedUser *acl.AuthenticatedUser
	EntityPrefix      string

	logEntry           *InternalLogEntry
	finalParsedCommand string
	executor           *Executor
}

// InternalLogEntry objects are created by an Executor, and represent the final
// state of execution (even if the command is not executed). It's designed to be
// easily serializable.
type InternalLogEntry struct {
	DatetimeStarted     time.Time
	DatetimeFinished    time.Time
	Output              string
	TimedOut            bool
	Blocked             bool
	ExitCode            int32
	Tags                []string
	ExecutionStarted    bool
	ExecutionFinished   bool
	ExecutionTrackingID string
	Process             *os.Process

	/*
		The following 3 properties are obviously on Action normally, but it's useful
		that logs are lightweight (so we don't need to have an action associated to
		logs, etc. Therefore, we duplicate those values here.
	*/
	ActionTitle string
	ActionIcon  string
	ActionId    string
}

type executorStepFunc func(*ExecutionRequest) bool

// DefaultExecutor returns an Executor, with a sensible "chain of command" for
// executing actions.
func DefaultExecutor(cfg *config.Config) *Executor {
	e := Executor{}
	e.Cfg = cfg
	e.Logs = make(map[string]*InternalLogEntry)
	e.LogsByActionId = make(map[string][]*InternalLogEntry)
	e.MapActionIdToBinding = make(map[string]*ActionBinding)

	e.chainOfCommand = []executorStepFunc{
		stepRequestAction,
		stepConcurrencyCheck,
		stepRateCheck,
		stepACLCheck,
		stepParseArgs,
		stepLogStart,
		stepExec,
		stepExecAfter,
		stepLogFinish,
		stepSaveLog,
		stepTrigger,
	}

	return &e
}

type listener interface {
	OnExecutionStarted(actionTitle string)
	OnExecutionFinished(logEntry *InternalLogEntry)
	OnOutputChunk(o []byte, executionTrackingId string)
	OnActionMapRebuilt()
}

func (e *Executor) AddListener(m listener) {
	e.listeners = append(e.listeners, m)
}

// ExecRequest processes an ExecutionRequest
func (e *Executor) ExecRequest(req *ExecutionRequest) (*sync.WaitGroup, string) {
	req.executor = e

	// req.UUID is now set by the client, so that they can track the request
	// from start to finish. This means that a malicious client could send
	// duplicate UUIDs (or just random strings), but this is the only way.

	req.logEntry = &InternalLogEntry{
		DatetimeStarted:     time.Now(),
		ExecutionTrackingID: req.TrackingID,
		Output:              "",
		ExitCode:            -1337, // If an Action is not actually executed, this is the default exit code.
		ExecutionStarted:    false,
		ExecutionFinished:   false,
		ActionId:            "",
		ActionTitle:         "notfound",
		ActionIcon:          "&#x1f4a9;",
	}

	_, foundLog := e.Logs[req.TrackingID]

	if foundLog || req.TrackingID == "" {
		req.TrackingID = uuid.NewString()
	}

	e.Logs[req.TrackingID] = req.logEntry

	wg := new(sync.WaitGroup)
	wg.Add(1)

	go func() {
		e.execChain(req)
		defer wg.Done()
	}()

	return wg, req.TrackingID
}

func (e *Executor) execChain(req *ExecutionRequest) {
	for _, step := range e.chainOfCommand {
		if !step(req) {
			break
		}
	}

	req.logEntry.ExecutionFinished = true

	// This isn't a step, because we want to notify all listeners, irrespective
	// of how many steps were actually executed.
	notifyListeners(req)
}

func getConcurrentCount(req *ExecutionRequest) int {
	concurrentCount := 0

	for _, log := range req.executor.LogsByActionId[req.Action.ID] {
		if !log.ExecutionFinished {
			concurrentCount += 1
		}
	}

	return concurrentCount
}

func stepConcurrencyCheck(req *ExecutionRequest) bool {
	concurrentCount := getConcurrentCount(req)

	// Note that the current execution is counted int the logs, so when checking we +1
	if concurrentCount >= (req.Action.MaxConcurrent + 1) {
		msg := fmt.Sprintf("Blocked from executing. This would mean this action is running %d times concurrently, but this action has maxExecutions set to %d.", concurrentCount, req.Action.MaxConcurrent)

		log.WithFields(log.Fields{
			"actionTitle": req.logEntry.ActionTitle,
		}).Warnf(msg)

		req.logEntry.Output = msg
		req.logEntry.Blocked = true
		return false
	}

	return true
}

func parseDuration(rate config.RateSpec) time.Duration {
	duration, err := time.ParseDuration(rate.Duration)

	if err != nil {
		log.Warnf("Could not parse duration: %v", rate.Duration)

		return -1 * time.Minute
	}

	return duration
}

func getExecutionsCount(rate config.RateSpec, req *ExecutionRequest) int {
	executions := -1 // Because we will find ourself when checking execution logs

	duration := parseDuration(rate)

	then := time.Now().Add(-duration)

	for _, logEntry := range req.executor.LogsByActionId[req.Action.ID] {
		if logEntry.DatetimeStarted.After(then) && !logEntry.Blocked {

			executions += 1
		}
	}

	return executions
}

func stepRateCheck(req *ExecutionRequest) bool {
	for _, rate := range req.Action.MaxRate {
		executions := getExecutionsCount(rate, req)

		if executions >= rate.Limit {
			msg := fmt.Sprintf("Blocked from executing. This action has run %d out of %d allowed times in the last %s.", executions, rate.Limit, rate.Duration)

			log.WithFields(log.Fields{
				"actionTitle": req.logEntry.ActionTitle,
			}).Infof(msg)

			req.logEntry.Output = msg
			req.logEntry.Blocked = true
			return false
		}
	}

	return true
}

func stepACLCheck(req *ExecutionRequest) bool {
	return acl.IsAllowedExec(req.Cfg, req.AuthenticatedUser, req.Action)
}

func stepParseArgs(req *ExecutionRequest) bool {
	var err error

	req.finalParsedCommand, err = parseActionArguments(req.Action.Shell, req.Arguments, req.Action, req.logEntry.ActionTitle, req.EntityPrefix)

	if err != nil {
		req.logEntry.Output = err.Error()

		log.Warnf(err.Error())

		return false
	}

	return true
}

func stepRequestAction(req *ExecutionRequest) bool {
	// The grpc API always tries to find the action by ID, but it may
	if req.Action == nil {
		log.WithFields(log.Fields{
			"actionTitle": req.ActionTitle,
		}).Infof("Action finding by title")

		req.Action = req.Cfg.FindAction(req.ActionTitle)

		if req.Action == nil {
			log.WithFields(log.Fields{
				"actionTitle": req.ActionTitle,
			}).Warnf("Action requested, but not found")

			req.logEntry.Output = "Action not found: " + req.ActionTitle

			return false
		}
	}

	metricActionsRequested.Inc()

	req.logEntry.ActionTitle = sv.ReplaceEntityVars(req.EntityPrefix, req.Action.Title)
	req.logEntry.ActionIcon = req.Action.Icon
	req.logEntry.ActionId = req.Action.ID

	if _, containsKey := req.executor.LogsByActionId[req.Action.ID]; !containsKey {
		req.executor.LogsByActionId[req.Action.ID] = make([]*InternalLogEntry, 0)
	}

	req.executor.LogsByActionId[req.Action.ID] = append(req.executor.LogsByActionId[req.Action.ID], req.logEntry)

	log.WithFields(log.Fields{
		"actionTitle": req.logEntry.ActionTitle,
		"tags":        req.Tags,
	}).Infof("Action requested")

	return true
}

func stepLogStart(req *ExecutionRequest) bool {
	log.WithFields(log.Fields{
		"actionTitle": req.logEntry.ActionTitle,
		"timeout":     req.Action.Timeout,
	}).Infof("Action starting")

	return true
}

func stepLogFinish(req *ExecutionRequest) bool {
	req.logEntry.ExecutionFinished = true

	log.WithFields(log.Fields{
		"actionTitle":  req.logEntry.ActionTitle,
		"outputLength": len(req.logEntry.Output),
		"timedOut":     req.logEntry.TimedOut,
		"exit":         req.logEntry.ExitCode,
	}).Infof("Action finished")

	return true
}

func notifyListeners(req *ExecutionRequest) {
	for _, listener := range req.executor.listeners {
		listener.OnExecutionFinished(req.logEntry)
	}
}

func appendErrorToStderr(err error, logEntry *InternalLogEntry) {
	if err != nil {
		logEntry.Output = err.Error() + "\n\n" + logEntry.Output
	}
}

type OutputStreamer struct {
	Req    *ExecutionRequest
	output bytes.Buffer
}

func (ost *OutputStreamer) Write(o []byte) (n int, err error) {
	for _, listener := range ost.Req.executor.listeners {
		listener.OnOutputChunk(o, ost.Req.TrackingID)
	}

	return ost.output.Write(o)
}

func (ost *OutputStreamer) String() string {
	return ost.output.String()
}

func buildEnv(req *ExecutionRequest) []string {
	ret := append(os.Environ(), "OLIVETIN=1")

	for k, v := range req.Arguments {
		ret = append(ret, fmt.Sprintf("%v=%v", strings.ToUpper(k), v))
	}

	return ret
}

func stepExec(req *ExecutionRequest) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.Action.Timeout)*time.Second)
	defer cancel()

	streamer := &OutputStreamer{Req: req}

	cmd := wrapCommandInShell(ctx, req.finalParsedCommand)
	cmd.Stdout = streamer
	cmd.Stderr = streamer
	cmd.Env = buildEnv(req)

	req.logEntry.ExecutionStarted = true

	runerr := cmd.Start()

	req.logEntry.Process = cmd.Process

	waiterr := cmd.Wait()

	req.logEntry.ExitCode = int32(cmd.ProcessState.ExitCode())
	req.logEntry.Output = streamer.String()

	appendErrorToStderr(runerr, req.logEntry)
	appendErrorToStderr(waiterr, req.logEntry)

	if ctx.Err() == context.DeadlineExceeded {
		// The context timeout should kill the process, but let's make sure.
		req.executor.Kill(req.logEntry)
		req.logEntry.TimedOut = true
	}

	req.logEntry.Tags = req.Tags
	req.logEntry.DatetimeFinished = time.Now()

	return true
}

func stepExecAfter(req *ExecutionRequest) bool {
	if req.Action.ShellAfterCompleted == "" {
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.Action.Timeout)*time.Second)
	defer cancel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	args := map[string]string{
		"output":   req.logEntry.Output,
		"exitCode": fmt.Sprintf("%v", req.logEntry.ExitCode),
	}

	finalParsedCommand, _ := parseActionArguments(req.Action.ShellAfterCompleted, args, req.Action, req.logEntry.ActionTitle, req.EntityPrefix)

	cmd := wrapCommandInShell(ctx, finalParsedCommand)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runerr := cmd.Start()

	waiterr := cmd.Wait()

	req.logEntry.Output += "---\n" + stdout.String()
	req.logEntry.Output += "---\n" + stderr.String()

	appendErrorToStderr(runerr, req.logEntry)
	appendErrorToStderr(waiterr, req.logEntry)

	if ctx.Err() == context.DeadlineExceeded {
		req.logEntry.Output += "Your shellAfterCommand command timed out."
	}

	req.logEntry.Output += fmt.Sprintf("Your shellAfterCommand exited with code %v", cmd.ProcessState.ExitCode())

	return true
}

func stepTrigger(req *ExecutionRequest) bool {
	if req.Action.Trigger != "" {
		trigger := &ExecutionRequest{
			ActionTitle:       req.Action.Trigger,
			TrackingID:        uuid.NewString(),
			Tags:              []string{"trigger"},
			AuthenticatedUser: req.AuthenticatedUser,
			Cfg:               req.Cfg,
		}

		req.executor.ExecRequest(trigger)
	}

	return true
}

func stepSaveLog(req *ExecutionRequest) bool {
	filename := fmt.Sprintf("%v.%v.%v", req.logEntry.ActionTitle, req.logEntry.DatetimeStarted.Unix(), req.logEntry.ExecutionTrackingID)

	saveLogResults(req, filename)
	saveLogOutput(req, filename)

	return true
}

func firstNonEmpty(one, two string) string {
	if one != "" {
		return one
	}

	return two
}

func saveLogResults(req *ExecutionRequest, filename string) {
	dir := firstNonEmpty(req.Action.SaveLogs.ResultsDirectory, req.Cfg.SaveLogs.ResultsDirectory)

	if dir != "" {
		data, err := yaml.Marshal(req.logEntry)

		if err != nil {
			log.Warnf("%v", err)
		}

		filepath := path.Join(dir, filename+".yaml")
		err = os.WriteFile(filepath, data, 0644)

		if err != nil {
			log.Warnf("%v", err)
		}
	}
}

func saveLogOutput(req *ExecutionRequest, filename string) {
	dir := firstNonEmpty(req.Action.SaveLogs.OutputDirectory, req.Cfg.SaveLogs.OutputDirectory)

	if dir != "" {
		data := req.logEntry.Output
		filepath := path.Join(dir, filename+".log")
		err := os.WriteFile(filepath, []byte(data), 0644)

		if err != nil {
			log.Warnf("%v", err)
		}
	}
}
