package config

import (
	"fmt"
)

// Action represents the core functionality of OliveTin - commands that show up
// as buttons in the UI.
type Action struct {
	ID                     string
	Title                  string
	Icon                   string
	Shell                  string
	ShellAfterCompleted    string
	Timeout                int
	Acls                   []string
	Entity                 string
	Hidden                 bool
	ExecOnStartup          bool
	ExecOnCron             []string
	ExecOnFileCreatedInDir []string
	ExecOnFileChangedInDir []string
	ExecOnCalendarFile     string
	Trigger                string
	MaxConcurrent          int
	MaxRate                []RateSpec
	Arguments              []ActionArgument
	PopupOnStart           string
	SaveLogs               SaveLogsConfig
}

// ActionArgument objects appear on Actions.
type ActionArgument struct {
	Name        string
	Title       string
	Description string
	Type        string
	Default     string
	Choices     []ActionArgumentChoice
	Entity      string
	RejectNull  bool
	Suggestions map[string]string
}

// ActionArgumentChoice represents a predefined choice for an argument.
type ActionArgumentChoice struct {
	Value string
	Title string
}

// RateSpec allows you to set a max frequency for an action.
type RateSpec struct {
	Limit    int
	Duration string
}

// Entity represents a "thing" that can have multiple actions associated with it.
// for example, a media player with a start and stop action.
type EntityFile struct {
	File string
	Name string
	Icon string
}

// PermissionsList defines what users can do with an action.
type PermissionsList struct {
	View bool
	Exec bool
	Logs bool
}

// AccessControlList defines what permissions apply to a user or user group.
type AccessControlList struct {
	Name             string
	AddToEveryAction bool
	MatchUsergroups  []string
	MatchUsernames   []string
	Permissions      PermissionsList
}

type PrometheusConfig struct {
	Enabled          bool
	DefaultGoMetrics bool
}

// Config is the global config used through the whole app.
type Config struct {
	UseSingleHTTPFrontend           bool
	ThemeName                       string
	ThemeCacheDisabled              bool
	ListenAddressSingleHTTPFrontend string
	ListenAddressWebUI              string
	ListenAddressRestActions        string
	ListenAddressGrpcActions        string
	ListenAddressPrometheus         string
	ExternalRestAddress             string
	LogLevel                        string
	LogDebugOptions                 LogDebugOptions
	LogHistoryPageSize              int64
	Actions                         []*Action             `mapstructure:"actions"`
	Entities                        []*EntityFile         `mapstructure:"entities"`
	Dashboards                      []*DashboardComponent `mapstructure:"dashboards"`
	CheckForUpdates                 bool
	PageTitle                       string
	ShowFooter                      bool
	ShowNavigation                  bool
	ShowNewVersions                 bool
	EnableCustomJs                  bool
	AuthJwtCookieName               string
	AuthJwtAud                      string
	AuthJwtDomain                   string
	AuthJwtCertsURL                 string
	AuthJwtHmacSecret               string // mutually exclusive with pub key config fields
	AuthJwtClaimUsername            string
	AuthJwtClaimUserGroup           string
	AuthJwtPubKeyPath               string // will read pub key from file on disk
	AuthHttpHeaderUsername          string
	AuthHttpHeaderUserGroup         string
	AuthLocalUsers                  AuthLocalUsersConfig
	AuthLoginUrl                    string
	AuthRequireGuestsToLogin        bool
	AuthOAuth2RedirectURL           string
	AuthOAuth2Providers             map[string]*OAuth2Provider
	DefaultPermissions              PermissionsList
	AccessControlLists              []*AccessControlList
	WebUIDir                        string
	CronSupportForSeconds           bool
	SectionNavigationStyle          string
	DefaultPopupOnStart             string
	InsecureAllowDumpVars           bool
	InsecureAllowDumpSos            bool
	InsecureAllowDumpActionMap      bool
	InsecureAllowDumpJwtClaims      bool
	Prometheus                      PrometheusConfig
	SaveLogs                        SaveLogsConfig
	DefaultIconForActions           string
	DefaultIconForDirectories       string
	DefaultIconForBack              string
	AdditionalNavigationLinks       []*NavigationLink

	usedConfigDir string
}

type AuthLocalUsersConfig struct {
	Enabled bool
	Users   []*LocalUser
}

type LocalUser struct {
	Username string
	Usergroup string
	Password string
}

type OAuth2Provider struct {
	Name               string
	Title              string
	ClientID           string
	ClientSecret       string
	Icon               string
	Scopes             []string
	AuthUrl            string
	TokenUrl           string
	WhoamiUrl          string
	UsernameField      string
	UserGroupField     string
	InsecureSkipVerify bool
	CallbackTimeout    int
	CertBundlePath     string
}

type NavigationLink struct {
	Title  string
	Url    string
	Target string
}

type SaveLogsConfig struct {
	ResultsDirectory string
	OutputDirectory  string
}

type LogDebugOptions struct {
	SingleFrontendRequests       bool
	SingleFrontendRequestHeaders bool
	AclCheckStarted              bool
	AclMatched                   bool
	AclNotMatched                bool
	AclNoneMatched               bool
}

type DashboardComponent struct {
	Title    string
	Type     string
	Entity   string
	Icon     string
	CssClass string
	Contents []DashboardComponent
}

func DefaultConfig() *Config {
	return DefaultConfigWithBasePort(1337)
}

// DefaultConfig gets a new Config structure with sensible default values.
func DefaultConfigWithBasePort(basePort int) *Config {
	config := Config{}
	config.UseSingleHTTPFrontend = true
	config.PageTitle = "OliveTin"
	config.ShowFooter = true
	config.ShowNavigation = true
	config.ShowNewVersions = true
	config.EnableCustomJs = false
	config.ExternalRestAddress = "."
	config.LogLevel = "INFO"
	config.LogHistoryPageSize = 10
	config.CheckForUpdates = false
	config.DefaultPermissions.Exec = true
	config.DefaultPermissions.View = true
	config.DefaultPermissions.Logs = true
	config.AuthJwtClaimUsername = "name"
	config.AuthJwtClaimUserGroup = "group"
	config.AuthRequireGuestsToLogin = false
	config.WebUIDir = "./webui"
	config.CronSupportForSeconds = false
	config.SectionNavigationStyle = "sidebar"
	config.DefaultPopupOnStart = "nothing"
	config.InsecureAllowDumpVars = false
	config.InsecureAllowDumpSos = false
	config.InsecureAllowDumpActionMap = false
	config.InsecureAllowDumpJwtClaims = false
	config.Prometheus.Enabled = false
	config.Prometheus.DefaultGoMetrics = false
	config.DefaultIconForActions = "&#x1F600;"
	config.DefaultIconForDirectories = "&#128193"
	config.DefaultIconForBack = "&laquo;"
	config.ThemeCacheDisabled = false

	config.ListenAddressSingleHTTPFrontend = fmt.Sprintf("0.0.0.0:%d", basePort)
	config.ListenAddressRestActions = fmt.Sprintf("localhost:%d", basePort+1)
	config.ListenAddressGrpcActions = fmt.Sprintf("localhost:%d", basePort+2)
	config.ListenAddressWebUI = fmt.Sprintf("localhost:%d", basePort+3)
	config.ListenAddressPrometheus = fmt.Sprintf("localhost:%d", basePort+4)

	return &config
}
