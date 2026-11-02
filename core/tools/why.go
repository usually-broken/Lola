package tools

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ErrNoReason is returned when a tool call arrives without a reason.
//
// Every call has to say why it is being made, in one line, in the caller's own
// words. That line is what a person is shown when the call needs approving, and
// what voice can speak before the result exists. A call with no reason is refused
// rather than guessed at, because an approval without a reason is a decision made
// on a command instead of on an intention.
var ErrNoReason = errors.New("every tool call needs a why")

// reasonField is the argument every tool carries, whatever else it carries.
const reasonField = "why"

// WithWhy adds the required reason to a tool's schema.
//
// It is applied by whatever builds the tool list, rather than by each plugin, so
// a tool written next month cannot forget to ask for one.
func WithWhy(schema json.RawMessage) json.RawMessage {
	var doc map[string]any
	if err := json.Unmarshal(schema, &doc); err != nil || doc == nil {
		// A schema this cannot read is left alone rather than replaced with a
		// guess: a tool with an odd schema still works, and this is only about
		// asking for the reason.
		return schema
	}

	props, ok := doc["properties"].(map[string]any)
	if !ok {
		props = map[string]any{}
		doc["properties"] = props
	}

	props[reasonField] = map[string]any{
		"type":        "string",
		"description": "One short line, in your own words, saying what you are about to do and what for. Shown to the person approving it.",
	}

	required, _ := doc["required"].([]any)
	for _, r := range required {
		if s, _ := r.(string); s == reasonField {
			return schema // already asked for
		}
	}
	doc["required"] = append(required, reasonField)

	out, err := json.Marshal(doc)
	if err != nil {
		return schema
	}
	return out
}

// Reason pulls the reason out of a call's arguments, and says plainly when there
// isn't one.
func Reason(args json.RawMessage) (string, error) {
	args = unwrapArgs(args)
	if len(args) == 0 {
		return "", ErrNoReason
	}

	var probe map[string]any
	if err := json.Unmarshal(args, &probe); err != nil {
		return "", ErrNoReason
	}

	raw, ok := probe[reasonField]
	if !ok {
		return "", ErrNoReason
	}

	reason, _ := raw.(string)
	if trimSpace(reason) == "" {
		return "", fmt.Errorf("%w: the one you sent was empty", ErrNoReason)
	}
	return reason, nil
}

// trimSpace is strings.TrimSpace without the import, kept local so this file says
// exactly what it does.
func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// ReasonOrArgs is what a frontend shows for a tool call: the caller's reason if
// there is one, and the arguments if there is not. The argument blob is for the
// tool; the sentence is for the person.
func ReasonOrArgs(args json.RawMessage) string {
	if reason, err := Reason(args); err == nil {
		return reason
	}
	args = unwrapArgs(args)
	if len(args) == 0 {
		return ""
	}
	return string(args)
}
