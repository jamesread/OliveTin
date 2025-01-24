package grpcapi

import (
	ctx "context"

	pb "github.com/OliveTin/OliveTin/gen/grpc"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/api/httpbody"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"errors"
	"net"

	acl "github.com/OliveTin/OliveTin/internal/acl"
	config "github.com/OliveTin/OliveTin/internal/config"
	executor "github.com/OliveTin/OliveTin/internal/executor"
	installationinfo "github.com/OliveTin/OliveTin/internal/installationinfo"
	sv "github.com/OliveTin/OliveTin/internal/stringvariables"
)

var (
	cfg *config.Config
)

type oliveTinAPI struct {
	// Uncomment this if you want to allow undefined methods during dev.
	//	pb.UnimplementedOliveTinApiServiceServer

	executor *executor.Executor
}

func (api *oliveTinAPI) KillAction(ctx ctx.Context, req *pb.KillActionRequest) (*pb.KillActionResponse, error) {
	ret := &pb.KillActionResponse{
		ExecutionTrackingId: req.ExecutionTrackingId,
	}

	execReqLogEntry, found := api.executor.GetLog(req.ExecutionTrackingId)

	ret.Found = found

	if found {
		log.Warnf("Killing execution request by tracking ID: %v", req.ExecutionTrackingId)

		err := api.executor.Kill(execReqLogEntry)

		if err != nil {
			log.Warnf("Killing execution request err: %v", err)
			ret.AlreadyCompleted = true
			ret.Killed = false
		} else {
			ret.Killed = true
		}
	} else {
		log.Warnf("Killing execution request not possible - not found by tracking ID: %v", req.ExecutionTrackingId)
	}

	return ret, nil
}

func (api *oliveTinAPI) StartAction(ctx ctx.Context, req *pb.StartActionRequest) (*pb.StartActionResponse, error) {
	args := make(map[string]string)

	for _, arg := range req.Arguments {
		args[arg.Name] = arg.Value
	}

	api.executor.MapActionIdToBindingLock.RLock()
	pair := api.executor.MapActionIdToBinding[req.ActionId]
	api.executor.MapActionIdToBindingLock.RUnlock()

	if pair == nil || pair.Action == nil {
		return nil, status.Errorf(codes.NotFound, "Action not found.")
	}

	authenticatedUser := acl.UserFromContext(ctx, cfg)

	execReq := executor.ExecutionRequest{
		Action:            pair.Action,
		EntityPrefix:      pair.EntityPrefix,
		TrackingID:        req.UniqueTrackingId,
		Arguments:         args,
		AuthenticatedUser: authenticatedUser,
		Cfg:               cfg,
	}

	api.executor.ExecRequest(&execReq)

	return &pb.StartActionResponse{
		ExecutionTrackingId: execReq.TrackingID,
	}, nil
}

func (api *oliveTinAPI) PasswordHash(ctx ctx.Context, req *pb.PasswordHashRequest) (*httpbody.HttpBody, error) {
	hash, err := createHash(req.Password)

	if err != nil {
		return nil, status.Errorf(codes.Internal, "Error creating hash.")
	}

	ret := &httpbody.HttpBody{
		ContentType: "text/plain",
		Data:        []byte("Your password hash is: " + hash),
	}

	return ret, nil
}

func (api *oliveTinAPI) LocalUserLogin(ctx ctx.Context, req *pb.LocalUserLoginRequest) (*pb.LocalUserLoginResponse, error) {
	match := checkUserPassword(cfg, req.Username, req.Password)

	if match {
		grpc.SendHeader(ctx, metadata.Pairs("set-username", req.Username))

		log.WithFields(log.Fields{
			"username": req.Username,
		}).Info("LocalUserLogin: User logged in successfully.")
	} else {
		log.WithFields(log.Fields{
			"username": req.Username,
		}).Warn("LocalUserLogin: User login failed.")
	}

	return &pb.LocalUserLoginResponse{
		Success: match,
	}, nil
}

func (api *oliveTinAPI) StartActionAndWait(ctx ctx.Context, req *pb.StartActionAndWaitRequest) (*pb.StartActionAndWaitResponse, error) {
	args := make(map[string]string)

	for _, arg := range req.Arguments {
		args[arg.Name] = arg.Value
	}

	execReq := executor.ExecutionRequest{
		Action:            api.executor.FindActionBindingByID(req.ActionId),
		TrackingID:        uuid.NewString(),
		Arguments:         args,
		AuthenticatedUser: acl.UserFromContext(ctx, cfg),
		Cfg:               cfg,
	}

	wg, _ := api.executor.ExecRequest(&execReq)
	wg.Wait()

	internalLogEntry, ok := api.executor.GetLog(execReq.TrackingID)

	if ok {
		return &pb.StartActionAndWaitResponse{
			LogEntry: internalLogEntryToPb(internalLogEntry),
		}, nil
	} else {
		return nil, errors.New("Execution not found!")
	}
}

func (api *oliveTinAPI) StartActionByGet(ctx ctx.Context, req *pb.StartActionByGetRequest) (*pb.StartActionByGetResponse, error) {
	args := make(map[string]string)

	execReq := executor.ExecutionRequest{
		Action:            api.executor.FindActionBindingByID(req.ActionId),
		TrackingID:        uuid.NewString(),
		Arguments:         args,
		AuthenticatedUser: acl.UserFromContext(ctx, cfg),
		Cfg:               cfg,
	}

	_, uniqueTrackingId := api.executor.ExecRequest(&execReq)

	return &pb.StartActionByGetResponse{
		ExecutionTrackingId: uniqueTrackingId,
	}, nil
}

func (api *oliveTinAPI) StartActionByGetAndWait(ctx ctx.Context, req *pb.StartActionByGetAndWaitRequest) (*pb.StartActionByGetAndWaitResponse, error) {
	args := make(map[string]string)

	execReq := executor.ExecutionRequest{
		Action:            api.executor.FindActionBindingByID(req.ActionId),
		TrackingID:        uuid.NewString(),
		Arguments:         args,
		AuthenticatedUser: acl.UserFromContext(ctx, cfg),
		Cfg:               cfg,
	}

	wg, _ := api.executor.ExecRequest(&execReq)
	wg.Wait()

	internalLogEntry, ok := api.executor.GetLog(execReq.TrackingID)

	if ok {
		return &pb.StartActionByGetAndWaitResponse{
			LogEntry: internalLogEntryToPb(internalLogEntry),
		}, nil
	} else {
		return nil, status.Errorf(codes.NotFound, "Execution not found.")
	}
}

func internalLogEntryToPb(logEntry *executor.InternalLogEntry) *pb.LogEntry {
	return &pb.LogEntry{
		ActionTitle:         logEntry.ActionTitle,
		ActionIcon:          logEntry.ActionIcon,
		ActionId:            logEntry.ActionId,
		DatetimeStarted:     logEntry.DatetimeStarted.Format("2006-01-02 15:04:05"),
		DatetimeFinished:    logEntry.DatetimeFinished.Format("2006-01-02 15:04:05"),
		DatetimeIndex:       logEntry.Index,
		Output:              logEntry.Output,
		TimedOut:            logEntry.TimedOut,
		Blocked:             logEntry.Blocked,
		ExitCode:            logEntry.ExitCode,
		Tags:                logEntry.Tags,
		ExecutionTrackingId: logEntry.ExecutionTrackingID,
		ExecutionStarted:    logEntry.ExecutionStarted,
		ExecutionFinished:   logEntry.ExecutionFinished,
		User:                logEntry.Username,
	}
}

func getExecutionStatusByTrackingID(api *oliveTinAPI, executionTrackingId string) *executor.InternalLogEntry {
	logEntry, ok := api.executor.GetLog(executionTrackingId)

	if !ok {
		return nil
	}

	return logEntry
}

func getMostRecentExecutionStatusById(api *oliveTinAPI, actionId string) *executor.InternalLogEntry {
	var ile *executor.InternalLogEntry

	logs := api.executor.GetLogsByActionId(actionId)

	if len(logs) == 0 {
		return nil
	} else {
		// Get last log entry
		ile = logs[len(logs)-1]
	}

	return ile
}

func (api *oliveTinAPI) ExecutionStatus(ctx ctx.Context, req *pb.ExecutionStatusRequest) (*pb.ExecutionStatusResponse, error) {
	res := &pb.ExecutionStatusResponse{}

	var ile *executor.InternalLogEntry

	if req.ExecutionTrackingId != "" {
		ile = getExecutionStatusByTrackingID(api, req.ExecutionTrackingId)

	} else {
		ile = getMostRecentExecutionStatusById(api, req.ActionId)
	}

	if ile == nil {
		return nil, status.Error(codes.NotFound, "Execution not found")
	} else {
		res.LogEntry = internalLogEntryToPb(ile)
	}

	return res, nil
}

/**
func (api *oliveTinAPI) WatchExecution(req *pb.WatchExecutionRequest, srv pb.OliveTinApi_WatchExecutionServer) error {
	log.Infof("Watch")

	if logEntry, ok := api.executor.Logs[req.ExecutionUuid]; !ok {
		log.Errorf("Execution not found: %v", req.ExecutionUuid)

		return nil
	} else {
		if logEntry.ExecutionStarted {
			for !logEntry.ExecutionCompleted {
				tmp := make([]byte, 256)

				red, err := io.ReadAtLeast(logEntry.StdoutBuffer, tmp, 1)

				log.Infof("%v %v", red, err)

				srv.Send(&pb.WatchExecutionUpdate{
					Update: string(tmp),
				})
			}
		}

		return nil
	}
}
*/

func (api *oliveTinAPI) Logout(ctx ctx.Context, req *pb.LogoutRequest) (*httpbody.HttpBody, error) {
	user := acl.UserFromContext(ctx, cfg)

	grpc.SendHeader(ctx, metadata.Pairs("logout-provider", user.Provider))
	grpc.SendHeader(ctx, metadata.Pairs("logout-sid", user.SID))

	return nil, nil
}

func (api *oliveTinAPI) GetDashboardComponents(ctx ctx.Context, req *pb.GetDashboardComponentsRequest) (*pb.GetDashboardComponentsResponse, error) {
	user := acl.UserFromContext(ctx, cfg)

	if user.IsGuest() && cfg.AuthRequireGuestsToLogin {
		return nil, status.Errorf(codes.PermissionDenied, "Guests are not allowed to access the dashboard.")
	}

	res := buildDashboardResponse(api.executor, cfg, user)

	if len(res.Actions) == 0 {
		log.WithFields(log.Fields{
			"username":         user.Username,
			"usergroup":        user.Usergroup,
			"provider":         user.Provider,
			"acls":             user.Acls,
			"availableActions": len(cfg.Actions),
		}).Warn("Zero actions found for user")
	}

	log.Tracef("GetDashboardComponents: %v", res)

	dashboardCfgToPb(res, cfg.Dashboards, cfg)

	return res, nil
}

func (api *oliveTinAPI) GetLogs(ctx ctx.Context, req *pb.GetLogsRequest) (*pb.GetLogsResponse, error) {
	user := acl.UserFromContext(ctx, cfg)

	ret := &pb.GetLogsResponse{}

	logEntries, countRemaining := api.executor.GetLogTrackingIds(req.StartOffset, cfg.LogHistoryPageSize)

	for _, logEntry := range logEntries {
		action := cfg.FindAction(logEntry.ActionTitle)

		if action == nil || acl.IsAllowedLogs(cfg, user, action) {
			pbLogEntry := internalLogEntryToPb(logEntry)

			ret.Logs = append(ret.Logs, pbLogEntry)
		}
	}

	ret.CountRemaining = countRemaining
	ret.PageSize = cfg.LogHistoryPageSize

	return ret, nil
}

/*
This function is ONLY a helper for the UI - the arguments are validated properly
on the StartAction -> Executor chain. This is here basically to provide helpful
error messages more quickly before starting the action.
*/
func (api *oliveTinAPI) ValidateArgumentType(ctx ctx.Context, req *pb.ValidateArgumentTypeRequest) (*pb.ValidateArgumentTypeResponse, error) {
	err := executor.TypeSafetyCheck("", req.Value, req.Type)
	desc := ""

	if err != nil {
		desc = err.Error()
	}

	return &pb.ValidateArgumentTypeResponse{
		Valid:       err == nil,
		Description: desc,
	}, nil
}

func (api *oliveTinAPI) WhoAmI(ctx ctx.Context, req *pb.WhoAmIRequest) (*pb.WhoAmIResponse, error) {
	user := acl.UserFromContext(ctx, cfg)

	res := &pb.WhoAmIResponse{
		AuthenticatedUser: user.Username,
		Usergroup:         user.Usergroup,
		Provider:          user.Provider,
		Sid:               user.SID,
		Acls:              user.Acls,
	}

	log.Warnf("usergroup: %v", user.Usergroup)

	return res, nil
}

func (api *oliveTinAPI) SosReport(ctx ctx.Context, req *pb.SosReportRequest) (*httpbody.HttpBody, error) {
	sos := installationinfo.GetSosReport()

	if !cfg.InsecureAllowDumpSos {
		log.Info(sos)
		sos = "Your SOS Report has been logged to OliveTin logs.\n\nIf you are in a safe network, you can temporarily set `insecureAllowDumpSos: true` in your config.yaml, restart OliveTin, and refresh this page - it will put the output directly in the browser."
	}

	ret := &httpbody.HttpBody{
		ContentType: "text/plain",
		Data:        []byte(sos),
	}

	return ret, nil
}

func (api *oliveTinAPI) DumpVars(ctx ctx.Context, req *pb.DumpVarsRequest) (*pb.DumpVarsResponse, error) {
	res := &pb.DumpVarsResponse{}

	if !cfg.InsecureAllowDumpVars {
		res.Alert = "Dumping variables is not allowed by default because it is insecure."

		return res, nil
	}

	res.Alert = "Dumping variables has been enabled in the configuration. Please set InsecureAllowDumpVars = false again after you don't need it anymore"
	res.Contents = sv.GetAll()

	return res, nil
}

func (api *oliveTinAPI) DumpPublicIdActionMap(ctx ctx.Context, req *pb.DumpPublicIdActionMapRequest) (*pb.DumpPublicIdActionMapResponse, error) {
	res := &pb.DumpPublicIdActionMapResponse{}
	res.Contents = make(map[string]*pb.ActionEntityPair)

	if !cfg.InsecureAllowDumpActionMap {
		res.Alert = "Dumping Public IDs is disallowed."

		return res, nil
	}

	api.executor.MapActionIdToBindingLock.RLock()

	for k, v := range api.executor.MapActionIdToBinding {
		res.Contents[k] = &pb.ActionEntityPair{
			ActionTitle:  v.Action.Title,
			EntityPrefix: v.EntityPrefix,
		}
	}

	api.executor.MapActionIdToBindingLock.RUnlock()

	res.Alert = "Dumping variables has been enabled in the configuration. Please set InsecureAllowDumpActionMap = false again after you don't need it anymore"

	return res, nil
}

func (api *oliveTinAPI) GetReadyz(ctx ctx.Context, req *pb.GetReadyzRequest) (*pb.GetReadyzResponse, error) {
	res := &pb.GetReadyzResponse{
		Status: "OK",
	}

	return res, nil
}

// Start will start the GRPC API.
func Start(globalConfig *config.Config, ex *executor.Executor) {
	cfg = globalConfig

	log.WithFields(log.Fields{
		"address": cfg.ListenAddressGrpcActions,
	}).Info("Starting gRPC API")

	lis, err := net.Listen("tcp", cfg.ListenAddressGrpcActions)

	if err != nil {
		log.Fatalf("Failed to listen - %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterOliveTinApiServiceServer(grpcServer, newServer(ex))

	err = grpcServer.Serve(lis)

	if err != nil {
		log.Fatalf("Could not start gRPC Server - %v", err)
	}
}

func newServer(ex *executor.Executor) *oliveTinAPI {
	server := oliveTinAPI{}
	server.executor = ex
	return &server
}
