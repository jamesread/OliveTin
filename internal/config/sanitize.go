package config

import (
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"strings"
)

// Sanitize will look for common configuration issues, and fix them. For example,
// populating undefined fields - name -> title, etc.
func (cfg *Config) Sanitize() {
	cfg.sanitizeLogLevel()

	// log.Infof("cfg %p", cfg)

	for idx := range cfg.Actions {
		cfg.Actions[idx].sanitize(cfg)
	}
}

func (cfg *Config) sanitizeLogLevel() {
	if logLevel, err := log.ParseLevel(cfg.LogLevel); err == nil {
		log.Info("Setting log level to ", logLevel)
		log.SetLevel(logLevel)
	}
}

func (action *Action) sanitize(cfg *Config) {
	if action.Timeout < 3 {
		action.Timeout = 3
	}

	action.ID = getActionID(action)
	action.Icon = lookupHTMLIcon(action.Icon, cfg.DefaultIconForActions)
	action.PopupOnStart = sanitizePopupOnStart(action.PopupOnStart, cfg)

	if action.MaxConcurrent < 1 {
		action.MaxConcurrent = 1
	}

	for idx := range action.Arguments {
		action.Arguments[idx].sanitize()
	}

	sanitizeAuthRequireGuestsToLogin(cfg)
	sanitizeLogHistoryPageSize(cfg)
}

func sanitizeAuthRequireGuestsToLogin(cfg *Config) {
	if cfg.AuthRequireGuestsToLogin {
		log.Infof("AuthRequireGuestsToLogin is enabled. All defaultPermissions will be set to false")

		cfg.DefaultPermissions.View = false
		cfg.DefaultPermissions.Exec = false
		cfg.DefaultPermissions.Logs = false
	}
}

func sanitizeLogHistoryPageSize(cfg *Config) {
	if cfg.LogHistoryPageSize < 10 {
		log.Warnf("LogsHistoryLimit is too low, setting it to 10")
		cfg.LogHistoryPageSize = 10
	} else if cfg.LogHistoryPageSize > 100 {
		log.Warnf("LogsHistoryLimit is high, you can do this, but expect browser lag.")
	}
}

func getActionID(action *Action) string {
	if action.ID == "" {
		return uuid.NewString()
	}

	if strings.Contains(action.ID, "{{") {
		log.Fatalf("Action IDs cannot contain variables")
	}

	return action.ID
}

func sanitizePopupOnStart(raw string, cfg *Config) string {
	switch raw {
	case "execution-dialog":
		return raw
	case "execution-dialog-stdout-only":
		return raw
	case "execution-button":
		return raw
	default:
		return cfg.DefaultPopupOnStart
	}
}

func (arg *ActionArgument) sanitize() {
	if arg.Title == "" {
		arg.Title = arg.Name
	}

	for idx, choice := range arg.Choices {
		if choice.Title == "" {
			arg.Choices[idx].Title = choice.Value
		}
	}

	arg.sanitizeNoType()

	// TODO Validate the default against the type checker, but this creates a
	// import loop
}

func (arg *ActionArgument) sanitizeNoType() {
	if len(arg.Choices) == 0 && arg.Type == "" {
		log.WithFields(log.Fields{
			"arg": arg.Name,
		}).Warn("Argument type isn't set, will default to 'ascii' but this may not be safe. You should set a type specifically.")
		arg.Type = "ascii"
	}
}
