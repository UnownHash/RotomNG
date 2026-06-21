package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// Custom slog.Level constants for extended severity levels.
const (
	LevelTrace slog.Level = -8 // below Debug(-4)
	LevelFatal slog.Level = 12 // above Error(8)
	LevelPanic slog.Level = 16 // above Fatal
)

// SlogHandlerOptions configures a SlogHandler.
type SlogHandlerOptions struct {
	// Level reports the minimum level to log. If nil, defaults to slog.LevelInfo.
	Level slog.Leveler
}

// groupOrAttrs holds either a group name or a list of pre-computed attributes.
type groupOrAttrs struct {
	group string
	attrs []slog.Attr
}

// SlogHandler is a custom slog.Handler that supports plain and JSON output formats,
// dynamic level control via slog.LevelVar, and concurrent-safe writes to any io.Writer.
type SlogHandler struct {
	opts   SlogHandlerOptions
	mu     *sync.Mutex // pointer, shared across WithAttrs/WithGroup copies
	out    io.Writer
	goa    []groupOrAttrs
	format string // "plain" or "json"
}

// NewSlogHandler creates a new SlogHandler writing to out in the specified format.
// If opts is nil, defaults to slog.LevelInfo.
func NewSlogHandler(out io.Writer, format string, opts *SlogHandlerOptions) *SlogHandler {
	h := &SlogHandler{
		mu:     &sync.Mutex{},
		out:    out,
		format: format,
	}
	if opts != nil {
		h.opts = *opts
	}
	if h.opts.Level == nil {
		h.opts.Level = slog.LevelInfo
	}
	return h
}

// Enabled reports whether the handler handles records at the given level.
func (h *SlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.opts.Level.Level()
}

// Handle formats and writes the log record to the handler's writer.
func (h *SlogHandler) Handle(_ context.Context, r slog.Record) error {
	switch h.format {
	case formatPlain:
		return h.handlePlain(r)
	case formatJSON:
		return h.handleJSON(r)
	default:
		return fmt.Errorf("unknown format: %s", h.format)
	}
}

// WithAttrs returns a new handler whose attributes include attrs.
func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	return h.clone(groupOrAttrs{attrs: attrs})
}

// WithGroup returns a new handler with the given group name prepended to subsequent attrs.
func (h *SlogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return h.clone(groupOrAttrs{group: name})
}

// clone returns a copy of the handler with an additional groupOrAttrs entry.
func (h *SlogHandler) clone(entry groupOrAttrs) *SlogHandler {
	return &SlogHandler{
		opts:   h.opts,
		mu:     h.mu,
		out:    h.out,
		goa:    append(slices.Clone(h.goa), entry),
		format: h.format,
	}
}

// flattenAttr resolves and flattens an attr, handling inline groups (empty key groups).
func flattenAttr(a slog.Attr) []slog.Attr {
	a.Value = a.Value.Resolve()
	// Skip empty attrs
	if a.Equal(slog.Attr{}) {
		return nil
	}
	// Inline group with empty key
	if a.Value.Kind() == slog.KindGroup && a.Key == "" {
		var result []slog.Attr
		for _, ga := range a.Value.Group() {
			result = append(result, flattenAttr(ga)...)
		}
		return result
	}
	return []slog.Attr{a}
}

// collectAttrs gathers all pre-computed attrs from goa and record attrs,
// applying group prefixes as needed.
func (h *SlogHandler) collectAttrs(r slog.Record) (component string, attrs []slog.Attr, groups []string) {
	component = fieldSystem
	var groupStack []string

	// Process pre-computed goa entries
	for _, entry := range h.goa {
		if entry.group != "" { //nolint:nestif // group/attr tree traversal is inherently nested
			groupStack = append(groupStack, entry.group)
		} else {
			for _, a := range entry.attrs {
				for _, fa := range flattenAttr(a) {
					if fa.Key == fieldComponent && len(groupStack) == 0 {
						if s := fa.Value.String(); s != "" {
							component = s
						}
						continue
					}
					if len(groupStack) > 0 {
						fa.Key = strings.Join(groupStack, ".") + "." + fa.Key
					}
					attrs = append(attrs, fa)
				}
			}
		}
	}

	// Save group stack for record attrs
	groups = groupStack

	// Process record attrs
	r.Attrs(func(a slog.Attr) bool {
		for _, fa := range flattenAttr(a) {
			if fa.Key == fieldComponent && len(groups) == 0 {
				if s := fa.Value.String(); s != "" {
					component = s
				}
				continue
			}
			if len(groups) > 0 {
				fa.Key = strings.Join(groups, ".") + "." + fa.Key
			}
			attrs = append(attrs, fa)
		}
		return true
	})

	return component, attrs, groups
}

// handlePlain formats the record as a plain text line.
func (h *SlogHandler) handlePlain(r slog.Record) error {
	buf := make([]byte, 0, 256)

	// Level
	buf = append(buf, slogLevelShortName(r.Level)...)
	buf = append(buf, ' ')

	// Timestamp
	if !r.Time.IsZero() {
		buf = append(buf, r.Time.Format(timeFormat)...)
		buf = append(buf, ' ')
	}

	component, attrs, _ := h.collectAttrs(r)

	// Component
	buf = append(buf, '[')
	buf = append(buf, component...)
	buf = append(buf, ']')
	buf = append(buf, ' ')

	// Attrs (sorted, excluding component)
	if len(attrs) > 0 {
		sort.Slice(attrs, func(i, j int) bool {
			return attrs[i].Key < attrs[j].Key
		})

		buf = append(buf, "-- "...)
		for i, a := range attrs {
			if i > 0 {
				buf = append(buf, ", "...)
			}
			buf = append(buf, a.Key...)
			buf = append(buf, '=')
			buf = appendAttrValuePlain(buf, a.Value)
		}
		buf = append(buf, " -- "...)
	}

	// Message
	buf = append(buf, r.Message...)
	buf = append(buf, '\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.out.Write(buf)
	if err != nil {
		return fmt.Errorf("writing plain log: %w", err)
	}
	return nil
}

// handleJSON formats the record as a JSON object, with a specific
// key ordering.
func (h *SlogHandler) handleJSON(r slog.Record) error {
	var buf bytes.Buffer

	buf.WriteString(`{"timestamp":`)
	if !r.Time.IsZero() {
		ts, _ := json.Marshal(r.Time.UTC().Format(time.RFC3339Nano)) //nolint:errchkjson // time.RFC3339 is safe
		buf.Write(ts)
	} else {
		buf.WriteString(`""`)
	}

	buf.WriteString(`,"level":`)
	lvl, _ := json.Marshal(slogLevelFullName(r.Level)) //nolint:errchkjson // known safe string
	buf.Write(lvl)

	buf.WriteString(`,"message":`)
	msg, _ := json.Marshal(r.Message) //nolint:errchkjson // string input is safe
	buf.Write(msg)

	component, attrs, groups := h.collectAttrsJSON(r)

	buf.WriteString(`,"fields":{"component":`)
	comp, _ := json.Marshal(component) //nolint:errchkjson // string input is safe
	buf.Write(comp)

	// Write attrs as JSON, handling groups as nested objects
	writeJSONAttrs(&buf, attrs, groups)

	buf.WriteString("}}\n")

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.out.Write(buf.Bytes())
	if err != nil {
		return fmt.Errorf("writing JSON log: %w", err)
	}
	return nil
}

// collectJSONAttrs collects a single attr into the jsonAttr list, handling inline groups.
func collectJSONAttrs(attrs []jsonAttr, a slog.Attr, groupStack []string) []jsonAttr {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return attrs
	}
	// Inline group with empty key
	if a.Value.Kind() == slog.KindGroup && a.Key == "" {
		for _, ga := range a.Value.Group() {
			attrs = collectJSONAttrs(attrs, ga, groupStack)
		}
		return attrs
	}
	attrs = append(attrs, jsonAttr{
		key:    a.Key,
		value:  a.Value,
		groups: slices.Clone(groupStack),
	})
	return attrs
}

// collectAttrsJSON is similar to collectAttrs but builds nested group structures for JSON.
func (h *SlogHandler) collectAttrsJSON(r slog.Record) (component string, attrs []jsonAttr, groups []string) {
	component = fieldSystem
	var groupStack []string

	for _, entry := range h.goa {
		if entry.group != "" {
			groupStack = append(groupStack, entry.group)
		} else {
			for _, a := range entry.attrs {
				// Check for component before collecting
				resolved := a
				resolved.Value = resolved.Value.Resolve()
				if resolved.Key == fieldComponent && len(groupStack) == 0 {
					if s := resolved.Value.String(); s != "" {
						component = s
					}
					continue
				}
				attrs = collectJSONAttrs(attrs, a, groupStack)
			}
		}
	}

	groups = groupStack

	r.Attrs(func(a slog.Attr) bool {
		resolved := a
		resolved.Value = resolved.Value.Resolve()
		if resolved.Key == fieldComponent && len(groups) == 0 {
			if s := resolved.Value.String(); s != "" {
				component = s
			}
			return true
		}
		attrs = collectJSONAttrs(attrs, a, groups)
		return true
	})

	return component, attrs, groups
}

type jsonAttr struct {
	key    string
	value  slog.Value
	groups []string
}

// writeJSONAttrs writes jsonAttr entries to buf, building nested objects for groups.
func writeJSONAttrs(buf *bytes.Buffer, attrs []jsonAttr, groups []string) {
	hasGroups := len(groups) > 0
	for _, a := range attrs {
		if len(a.groups) > 0 {
			hasGroups = true
			break
		}
	}

	if !hasGroups {
		// Simple flat attrs, sorted
		sort.Slice(attrs, func(i, j int) bool {
			return attrs[i].key < attrs[j].key
		})
		for _, a := range attrs {
			buf.WriteString(`,"`)
			buf.WriteString(a.key)
			buf.WriteString(`":`)
			writeJSONValue(buf, a.value)
		}
		return
	}

	// Build nested structure for grouped attrs
	type nestedNode struct {
		children map[string]*nestedNode
		value    *slog.Value
	}

	root := &nestedNode{children: make(map[string]*nestedNode)}

	for _, a := range attrs {
		cur := root
		for _, g := range a.groups {
			if cur.children[g] == nil {
				cur.children[g] = &nestedNode{children: make(map[string]*nestedNode)}
			}
			cur = cur.children[g]
		}
		v := a.value
		cur.children[a.key] = &nestedNode{children: make(map[string]*nestedNode), value: &v}
	}

	// Write root children as top-level JSON fields
	var writeNode func(buf *bytes.Buffer, n *nestedNode)
	writeNode = func(buf *bytes.Buffer, n *nestedNode) {
		keys := make([]string, 0, len(n.children))
		for k := range n.children {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		first := true
		for _, k := range keys {
			child := n.children[k]
			if !first {
				buf.WriteByte(',')
			}
			first = false
			buf.WriteByte('"')
			buf.WriteString(k)
			buf.WriteString(`":`)
			if child.value != nil && len(child.children) == 0 {
				writeJSONValue(buf, *child.value)
			} else if len(child.children) > 0 {
				buf.WriteByte('{')
				writeNode(buf, child)
				buf.WriteByte('}')
			} else {
				buf.WriteString("null")
			}
		}
	}

	// Write each top-level key
	keys := make([]string, 0, len(root.children))
	for k := range root.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		child := root.children[k]
		buf.WriteString(`,"`)
		buf.WriteString(k)
		buf.WriteString(`":`)
		if child.value != nil && len(child.children) == 0 {
			writeJSONValue(buf, *child.value)
		} else if len(child.children) > 0 {
			buf.WriteByte('{')
			writeNode(buf, child)
			buf.WriteByte('}')
		} else {
			buf.WriteString("null")
		}
	}
}

// writeJSONValue writes a slog.Value as JSON.
func writeJSONValue(buf *bytes.Buffer, v slog.Value) {
	v = v.Resolve()

	switch v.Kind() { //nolint:exhaustive // KindLogValuer is resolved above
	case slog.KindString:
		b, _ := json.Marshal(v.String()) //nolint:errchkjson // string is safe
		buf.Write(b)
	case slog.KindInt64:
		b, _ := json.Marshal(v.Int64()) //nolint:errchkjson // int64 is safe
		buf.Write(b)
	case slog.KindUint64:
		b, _ := json.Marshal(v.Uint64()) //nolint:errchkjson // uint64 is safe
		buf.Write(b)
	case slog.KindFloat64:
		fmt.Fprintf(buf, "%g", v.Float64())
	case slog.KindBool:
		b, _ := json.Marshal(v.Bool()) //nolint:errchkjson // bool is safe
		buf.Write(b)
	case slog.KindDuration:
		b, _ := json.Marshal(v.Duration().String()) //nolint:errchkjson // string is safe
		buf.Write(b)
	case slog.KindTime:
		b, _ := json.Marshal(v.Time().UTC().Format(time.RFC3339)) //nolint:errchkjson // string is safe
		buf.Write(b)
	case slog.KindGroup:
		buf.WriteByte('{')
		first := true
		for _, a := range v.Group() {
			a.Value = a.Value.Resolve()
			if !first {
				buf.WriteByte(',')
			}
			first = false
			buf.WriteByte('"')
			buf.WriteString(a.Key)
			buf.WriteString(`":`)
			writeJSONValue(buf, a.Value)
		}
		buf.WriteByte('}')
	case slog.KindAny:
		val := v.Any()
		if err, ok := val.(error); ok {
			b, _ := json.Marshal(err.Error()) //nolint:errchkjson // string is safe
			buf.Write(b)
		} else {
			b, err := json.Marshal(val)
			if err != nil {
				b, _ = json.Marshal(fmt.Sprintf("%+v", val)) //nolint:errchkjson // string is safe
			}
			buf.Write(b)
		}
	}
}

// appendAttrValuePlain appends a slog.Value in plain text format.
func appendAttrValuePlain(buf []byte, v slog.Value) []byte {
	return append(buf, v.String()...)
}

// slogLevelShortName maps a slog.Level to the short level name used by PlainFormatter.
func slogLevelShortName(level slog.Level) string {
	switch {
	case level >= LevelPanic:
		return levelPanicShort
	case level >= LevelFatal:
		return levelFatalShort
	case level >= slog.LevelError:
		return levelErrorShort
	case level >= slog.LevelWarn:
		return levelWarnShort
	case level >= slog.LevelInfo:
		return levelInfoShort
	default:
		return levelDebugShort
	}
}

// slogLevelFullName maps a slog.Level to the full level name used by JSONFormatter.
func slogLevelFullName(level slog.Level) string {
	switch {
	case level >= LevelPanic:
		return levelPanicFull
	case level >= LevelFatal:
		return levelFatalFull
	case level >= slog.LevelError:
		return levelErrorFull
	case level >= slog.LevelWarn:
		return levelWarningFull
	case level >= slog.LevelInfo:
		return levelInfoFull
	default:
		return levelDebugFull
	}
}
