package transmission

import (
	"fmt"
	"log/slog"
	"strings"

	"transmission-proxy/internal/jrpc"
	"transmission-proxy/internal/logger"
)

var (
	ErrUnknownMethod            = fmt.Errorf("unknown method")
	ErrTorrentLocationWrongType = fmt.Errorf("must be string")
	ErrTorrentForbiddenLocation = fmt.Errorf("forbidden location")
)

type IsBadArgument interface {
	GetBadArgument() string
}

type forbiddenField struct {
	name string
}

func (f *forbiddenField) GetBadArgument() string {
	return f.name
}

func (f *forbiddenField) Error() string {
	return "forbidden field"
}

func (f *forbiddenField) GetLoggableAttrs() []slog.Attr {
	return []slog.Attr{slog.String("field", f.name)}
}

type skippedField struct {
	field string
}

func (s *skippedField) Error() string {
	return "skipped field"
}

func (s *skippedField) GetBadArgument() string {
	return s.field
}

func (s *skippedField) GetLoggableAttrs() []slog.Attr {
	return []slog.Attr{slog.String("field", s.field)}
}

type RequestValidator interface {
	Validate(req *jrpc.Request) error
}

type ArgumentsValidator interface {
	Validate(args map[string]any) (err error, info []any)
}

type ArgumentValidator interface {
	Validate(key string, value any) error
}

type MethodsValidator struct {
	Methods map[string]ArgumentsValidator
}

func (p *MethodsValidator) Validate(req *jrpc.Request) error {
	if v, ok := p.Methods[req.Method]; ok {
		err, info := v.Validate(req.Arguments)
		for _, i := range info {
			if sf, ok := i.(skippedField); ok {
				slog.WarnContext(req.Context, "skip field from RPC request",
					slog.String("method", req.Method),
					slog.String("field", sf.field))
			} else if ba, ok := i.(IsBadArgument); ok {
				slog.WarnContext(req.Context, fmt.Sprintf("%v", i),
					slog.String("method", req.Method),
					slog.String("field", ba.GetBadArgument()))
			} else {
				slog.WarnContext(req.Context, fmt.Sprintf("%v", i), slog.String("method", req.Method))
			}
		}

		return logger.WithAttributes(err, slog.String("method", req.Method))
	}

	return logger.WithAttributes(ErrUnknownMethod, slog.String("method", req.Method))
}

func DefaultMethodsValidator(requiredLocPrefix string) *MethodsValidator {
	return &MethodsValidator{Methods: map[string]ArgumentsValidator{
		"torrent-start":        &MethodTorrentAction,
		"torrent-start-now":    &MethodTorrentAction,
		"torrent-stop":         &MethodTorrentAction,
		"torrent-verify":       &MethodTorrentAction,
		"torrent-reannounce":   &MethodTorrentAction,
		"torrent-set":          NewMethodTorrentSet(requiredLocPrefix),
		"torrent-get":          &MethodTorrentGet,
		"torrent-add":          NewMethodTorrentAdd(requiredLocPrefix),
		"torrent-remove":       &MethodTorrentRemove,
		"torrent-set-location": NewMethodTorrentSetLocation(requiredLocPrefix),
		"session-set":          NewMethodSessionSet(requiredLocPrefix),
		"session-get":          &MethodSessionGet,
		"session-stats":        &EmptyMethod,
		"blocklist-update":     &EmptyMethod,
		"port-test":            &MethodPortTest,
		"session-close":        &EmptyMethod,
		"queue-move-top":       &MethodTorrentAction,
		"queue-move-up":        &MethodTorrentAction,
		"queue-move-down":      &MethodTorrentAction,
		"queue-move-bottom":    &MethodTorrentAction,
		"free-space":           &MethodFreeSpace,
		"group-set":            &MethodGroupSet,
		"group-get":            &MethodGroupGet,
	}}
}

type MethodArgumentsValidator struct {
	Arguments      map[string]ArgumentValidator
	ErrorOnUnknown bool
}

func (a *MethodArgumentsValidator) Validate(args map[string]any) (err error, info []any) {
	for key, val := range args {
		if v, ok := a.Arguments[key]; ok {
			if err := v.Validate(key, val); err != nil {
				return logger.WithAttributes(
					fmt.Errorf("bad argument: %w", err), slog.String("field", key),
				), info
			}
		} else if a.ErrorOnUnknown {
			return &forbiddenField{name: key}, info
		} else {
			info = append(info, skippedField{field: key})
			delete(args, key)
		}
	}

	return nil, info
}

type Any struct{}

func (a *Any) Validate(key string, value any) error {
	return nil
}

var EmptyMethod = MethodArgumentsValidator{Arguments: map[string]ArgumentValidator{}}

var MethodTorrentAction = MethodArgumentsValidator{Arguments: map[string]ArgumentValidator{
	"ids": &Any{},
}}

func NewMethodTorrentSet(requiredLocPrefix string) *MethodArgumentsValidator {
	return &MethodArgumentsValidator{Arguments: map[string]ArgumentValidator{
		"bandwidthPriority":           &Any{},
		"downloadLimit":               &Any{},
		"downloadLimited":             &Any{},
		"files-unwanted":              &Any{},
		"files-wanted":                &Any{},
		"group":                       &Any{},
		"honorsSessionLimit: &Any{}s": &Any{},
		"ids":                         &Any{},
		"labels":                      &Any{},
		"location":                    &PrefixedLocation{RequiredPrefix: requiredLocPrefix},
		"peer-limit":                  &Any{},
		"priority-high":               &Any{},
		"priority-low":                &Any{},
		"priority-normal":             &Any{},
		"queuePosition":               &Any{},
		"seedIdleLimit":               &Any{},
		"seedIdleMode":                &Any{},
		"seedRatioLimit":              &Any{},
		"seedRatioMode":               &Any{},
		"sequentialDownload":          &Any{},
		"trackerList":                 &Any{},
		"uploadLimit":                 &Any{},
		"uploadLimited":               &Any{},
	}}
}

type PrefixedLocation struct {
	RequiredPrefix string
}

func (t *PrefixedLocation) Validate(key string, value any) error {
	if loc, ok := value.(string); ok {
		if !strings.HasPrefix(loc, t.RequiredPrefix) {
			return ErrTorrentForbiddenLocation
		}

		return nil
	}

	return ErrTorrentLocationWrongType
}

var MethodTorrentGet = MethodArgumentsValidator{Arguments: map[string]ArgumentValidator{
	"ids":    &Any{},
	"fields": &Any{},
	"format": &Any{},
}}

func NewMethodTorrentAdd(requiredLocPrefix string) *MethodArgumentsValidator {
	return &MethodArgumentsValidator{Arguments: map[string]ArgumentValidator{
		"cookies":           &Any{},
		"download-dir":      &PrefixedLocation{RequiredPrefix: requiredLocPrefix},
		"filename":          &Any{},
		"labels":            &Any{},
		"metainfo":          &Any{},
		"paused":            &Any{},
		"peer-limit":        &Any{},
		"bandwidthPriority": &Any{},
		"files-wanted":      &Any{},
		"files-unwanted":    &Any{},
		"priority-high":     &Any{},
		"priority-low":      &Any{},
		"priority-normal":   &Any{},
	}}
}

var MethodTorrentRemove = MethodArgumentsValidator{Arguments: map[string]ArgumentValidator{
	"ids":               &Any{},
	"delete-local-data": &Any{},
}}

func NewMethodTorrentSetLocation(requiredLocPrefix string) *MethodArgumentsValidator {
	return &MethodArgumentsValidator{Arguments: map[string]ArgumentValidator{
		"ids":      &Any{},
		"location": &PrefixedLocation{RequiredPrefix: requiredLocPrefix},
		"move":     &Any{},
	}}
}

func NewMethodSessionSet(requiredLocPrefix string) *MethodArgumentsValidator {
	return &MethodArgumentsValidator{Arguments: map[string]ArgumentValidator{
		"alt-speed-down":             &Any{},
		"alt-speed-enabled":          &Any{},
		"alt-speed-time-begin":       &Any{},
		"alt-speed-time-day":         &Any{},
		"alt-speed-time-enabled":     &Any{},
		"alt-speed-time-end":         &Any{},
		"alt-speed-up":               &Any{},
		"blocklist-enabled":          &Any{},
		"blocklist-url":              &Any{},
		"cache-size-mb":              &Any{},
		"default-trackers":           &Any{},
		"dht-enabled":                &Any{},
		"download-dir":               &PrefixedLocation{RequiredPrefix: requiredLocPrefix},
		"download-queue-enabled":     &Any{},
		"download-queue-size":        &Any{},
		"encryption":                 &Any{},
		"idle-seeding-limit-enabled": &Any{},
		"idle-seeding-limit":         &Any{},
		//"incomplete-dir-enabled":               &Any{},
		//"incomplete-dir":                       &Any{},
		"lpd-enabled":            &Any{},
		"peer-limit-global":      &Any{},
		"peer-limit-per-torrent": &Any{},
		//"peer-port-random-on-start":            &Any{},
		//"peer-port":                            &Any{},
		"pex-enabled":             &Any{},
		"port-forwarding-enabled": &Any{},
		"queue-stalled-enabled":   &Any{},
		"queue-stalled-minutes":   &Any{},
		"rename-partial-files":    &Any{},
		//"script-torrent-added-enabled":         &Any{},
		//"script-torrent-added-filename":        &Any{},
		//"script-torrent-done-enabled":          &Any{},
		//"script-torrent-done-filename":         &Any{},
		//"script-torrent-done-seeding-enabled":  &Any{},
		//"script-torrent-done-seeding-filename": &Any{},
		"seed-queue-enabled":           &Any{},
		"seed-queue-size":              &Any{},
		"seedRatioLimit":               &Any{},
		"seedRatioLimited":             &Any{},
		"speed-limit-down-enabled":     &Any{},
		"speed-limit-down":             &Any{},
		"speed-limit-up-enabled":       &Any{},
		"speed-limit-up":               &Any{},
		"start-added-torrents":         &Any{},
		"trash-original-torrent-files": &Any{},
		"utp-enabled":                  &Any{},
	}}
}

var MethodSessionGet = MethodArgumentsValidator{Arguments: map[string]ArgumentValidator{
	"fields": &Any{},
}}

var MethodPortTest = MethodArgumentsValidator{Arguments: map[string]ArgumentValidator{
	"ipProtocol": &Any{},
}}

var MethodFreeSpace = MethodArgumentsValidator{Arguments: map[string]ArgumentValidator{
	"path": &Any{},
}}

var MethodGroupSet = MethodArgumentsValidator{Arguments: map[string]ArgumentValidator{
	"honorsSessionLimits":      &Any{},
	"name":                     &Any{},
	"speed-limit-down-enabled": &Any{},
	"speed-limit-down":         &Any{},
	"speed-limit-up-enabled":   &Any{},
	"speed-limit-up":           &Any{},
}}

var MethodGroupGet = MethodArgumentsValidator{Arguments: map[string]ArgumentValidator{
	"group": &Any{},
}}
