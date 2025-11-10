package logging

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/samber/oops"
	"go.elastic.co/ecszerolog"
)

type LoggerFactory struct {
	root  zerolog.Logger
	level levelTable
}

type levelTable struct {
	literal map[string]zerolog.Level
	regex   []regexRule
}

type regexRule struct {
	regexp *regexp.Regexp
	level  zerolog.Level
}

type Options struct {
	AppInstanceID string
	AppVersion    string
	AppCommit     string
	AppBuildDate  string
	RootLevel     string
	LiteralLevels map[string]string
	RegexLevels   map[string]string
	PrettyPrint   bool
}

func NewFactory(options *Options) (*LoggerFactory, error) {
	errorBuilder := oops.
		In("loggers factory").
		Tags("constructor")

	rootLevel, err := zerolog.ParseLevel(options.RootLevel)
	if err != nil {
		return nil, errorBuilder.Wrapf(err, "error parsing rootLevel '%s'", options.RootLevel)
	}

	var logContext zerolog.Context
	if options.PrettyPrint {
		logContext = zerolog.New(zerolog.ConsoleWriter{
			Out:           os.Stdout,
			TimeFormat:    time.RFC3339,
			TimeLocation:  time.UTC,
			PartsOrder:    []string{"time", "logger", "level", "message", "fields"},
			FieldsExclude: []string{"app-build-date", "app-commit", "app-version", "app-instance", "logger"},
			FormatTimestamp: func(ts any) string {
				return "\033[90m" + ts.(string) + "\033[0m" //nolint:errcheck,forcetypeassert // we know ts is string
			},
			FormatLevel: func(level any) string {
				level = strings.ToUpper(level.(string)) //nolint:errcheck,forcetypeassert // we know level is string
				var color string
				switch level {
				case "DEBUG":
					color = "\033[1;36m" // cyan
				case "INFO":
					color = "\033[1;32m" // green
				case "WARN":
					color = "\033[1;33m" // yellow
				case "ERROR":
					color = "\033[1;31m" // red
				case "FATAL":
					color = "\033[1;35m" // magenta
				default:
					color = "\033[1m"
				}
				s := fmt.Sprintf("%s%-5s\033[0m", color, level)
				return s
			},
			FormatCaller: func(i any) string {
				return fmt.Sprintf("\033[90m%s\033[0m", i)
			},
			FormatMessage: func(i any) string {
				return fmt.Sprintf(": %v", i)
			},
			FormatFieldName: func(i any) string {
				return fmt.Sprintf("\033[1m%s\033[0m=", i)
			},
			FormatFieldValue: func(i any) string {
				switch itype := i.(type) {
				case []byte:
					if isPrintable(itype) {
						return string(itype)
					}
					return fmt.Sprintf("%v", itype)
				default:
					return fmt.Sprintf("%v", itype)
				}
			},
			FormatPartValueByName: func(val any, part string) string {
				switch part {
				case "logger":
					s := fmt.Sprintf("\033[4;34m%s\033[0m", val)
					return fmt.Sprintf("[%-35s]", s)
				case "fields":
					// zerolog passes nil here; actual fields are printed separately.
					return ""
				default:
					return fmt.Sprint(val)
				}
			},
		}).
			With().
			Timestamp()
	} else {
		logContext = ecszerolog.New(os.Stdout).With()
	}

	registry := &LoggerFactory{
		root: logContext.
			Str("app-instance", options.AppInstanceID).
			Str("app-version", options.AppVersion).
			Str("app-commit", options.AppCommit).
			Str("app-build-date", options.AppBuildDate).
			Logger().
			Level(rootLevel),
		level: levelTable{
			literal: make(map[string]zerolog.Level),
		},
	}

	for literal, lvlStr := range options.LiteralLevels {
		lvl, err := zerolog.ParseLevel(lvlStr)
		if err != nil {
			return nil, errorBuilder.Wrapf(err, "error parsing level '%s' for literal '%s'", lvlStr, literal)
		}
		registry.level.literal[literal] = lvl
	}

	for pattern, lvlStr := range options.RegexLevels {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, errorBuilder.Wrapf(err, "error compiling regex pattern '%s'", pattern)
		}
		lvl, err := zerolog.ParseLevel(lvlStr)
		if err != nil {
			return nil, errorBuilder.Wrapf(err, "error parsing level '%s' for regex pattern '%s'", lvlStr, pattern)
		}
		registry.level.regex = append(registry.level.regex, regexRule{regexp: re, level: lvl})
	}

	return registry, nil
}

type LoggerOption func(ctx *zerolog.Context) zerolog.Context

func WithField(key string, value any) LoggerOption {
	return func(c *zerolog.Context) zerolog.Context {
		return c.Interface(key, value)
	}
}

func (lf *LoggerFactory) Child(name string, opts ...LoggerOption) zerolog.Logger {
	level := lf.getLevel(name)
	child := lf.root.With().Str("logger", name)

	for _, opt := range opts {
		child = opt(&child)
	}

	return child.Logger().Level(level)
}

func (lf *LoggerFactory) getLevel(name string) zerolog.Level {
	if lvl, ok := lf.level.literal[name]; ok {
		return lvl
	}

	for _, rule := range lf.level.regex {
		if rule.regexp.MatchString(name) {
			return rule.level
		}
	}

	return lf.root.GetLevel()
}

func isPrintable(b []byte) bool {
	for _, c := range b {
		// allow tab/newline and visible ASCII
		if (c < 32 && c != 9 && c != 10 && c != 13) || c > 126 {
			return false
		}
	}
	return true
}
