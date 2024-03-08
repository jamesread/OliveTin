package executor

import (
	config "github.com/OliveTin/OliveTin/internal/config"
	sv "github.com/OliveTin/OliveTin/internal/stringvariables"
	log "github.com/sirupsen/logrus"

	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	typecheckRegex = map[string]string{
		"very_dangerous_raw_string": "",
		"int":                       "^[\\d]+$",
		"ascii":                     "^[a-zA-Z0-9]+$",
		"ascii_identifier":          "^[a-zA-Z0-9\\-\\.\\_]+$",
		"ascii_sentence":            "^[a-zA-Z0-9 \\,\\.]+$",
	}
)

func parseActionArguments(rawShellCommand string, values map[string]string, action *config.Action, actionTitle string, entityPrefix string) (string, error) {
	log.WithFields(log.Fields{
		"actionTitle": actionTitle,
		"cmd":         rawShellCommand,
	}).Infof("Action parse args - Before")

	r := regexp.MustCompile("{{ *?([a-zA-Z0-9_]+?) *?}}")
	matches := r.FindAllStringSubmatch(rawShellCommand, -1)

	for _, match := range matches {
		argValue, argProvided := values[match[1]]

		if !argProvided {
			log.Infof("%v", values)
			return "", errors.New("Required arg not provided: " + match[1])
		}

		err := typecheckActionArgument(match[1], argValue, action)

		if err != nil {
			return "", err
		}

		log.WithFields(log.Fields{
			"name":  match[1],
			"value": argValue,
		}).Debugf("Arg assigned")

		rawShellCommand = strings.ReplaceAll(rawShellCommand, match[0], argValue)
	}

	rawShellCommand = sv.ReplaceEntityVars(entityPrefix, rawShellCommand)

	log.WithFields(log.Fields{
		"actionTitle": actionTitle,
		"cmd":         rawShellCommand,
	}).Infof("Action parse args - After")

	return rawShellCommand, nil
}

func typecheckActionArgument(name string, value string, action *config.Action) error {
	arg := action.FindArg(name)

	if arg == nil {
		return errors.New("Action arg not defined: " + name)
	}

	if len(arg.Choices) > 0 {
		return typecheckChoice(value, arg)
	}

	return TypeSafetyCheck(name, value, arg.Type)
}

func typecheckChoice(value string, arg *config.ActionArgument) error {
	for _, choice := range arg.Choices {
		if value == choice.Value {
			return nil
		}
	}

	return errors.New("argument value is not one of the predefined choices")
}

// TypeSafetyCheck checks argument values match a specific type. The types are
// defined in typecheckRegex, and, you guessed it, uses regex to check for allowed
// characters.
func TypeSafetyCheck(name string, value string, argumentType string) error {
	if argumentType == "url" {
		return typeSafetyCheckUrl(name, value)
	}

	if argumentType == "datetime" {
		_, err := time.Parse("2006-01-02T15:04:05", value)

		if err != nil {
			return err
		}

		return nil
	}

	return typeSafetyCheckRegex(name, value, argumentType)
}

func typeSafetyCheckRegex(name string, value string, argumentType string) error {
	pattern, found := typecheckRegex[argumentType]

	if !found {
		return errors.New("argument type not implemented " + argumentType)
	}

	matches, _ := regexp.MatchString(pattern, value)

	if !matches {
		log.WithFields(log.Fields{
			"name":  name,
			"value": value,
			"type":  argumentType,
		}).Warn("Arg type check safety failure")

		return errors.New("invalid argument, doesn't match " + argumentType)
	}

	return nil
}

func typeSafetyCheckUrl(name string, value string) error {
	_, err := url.ParseRequestURI(value)

	return err
}
