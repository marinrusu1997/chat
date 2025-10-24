package logging

import (
	"os"
	"regexp"

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
}

func NewFactory(options Options) (*LoggerFactory, error) {
	errorBuilder := oops.
		In("loggers factory").
		Tags("constructor")

	rootLevel, err := zerolog.ParseLevel(options.RootLevel)
	if err != nil {
		return nil, errorBuilder.Wrapf(err, "error parsing rootLevel '%s'", options.RootLevel)
	}

	registry := &LoggerFactory{
		root: ecszerolog.
			New(os.Stdout).
			With().
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

func WithField(key string, value interface{}) LoggerOption {
	return func(c *zerolog.Context) zerolog.Context {
		return c.Interface(key, value)
	}
}

func (registry *LoggerFactory) Child(name string, opts ...LoggerOption) zerolog.Logger {
	level := registry.getLevel(name)
	child := registry.root.With().Str("logger", name)

	for _, opt := range opts {
		child = opt(&child)
	}

	return child.Logger().Level(level)
}

func (registry *LoggerFactory) getLevel(name string) zerolog.Level {
	if lvl, ok := registry.level.literal[name]; ok {
		return lvl
	}

	for _, rule := range registry.level.regex {
		if rule.regexp.MatchString(name) {
			return rule.level
		}
	}

	return registry.root.GetLevel()
}
