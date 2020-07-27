// Copyright 2017 Andrew 'Diddymus' Rolfe. All rights reserved.
//
// Use of this source code is governed by the license in the LICENSE file
// included with the source code.

package attr

import (
	"log"
	"time"

	"code.wolfmud.org/WolfMUD.git/attr/internal"
	"code.wolfmud.org/WolfMUD.git/event"
	"code.wolfmud.org/WolfMUD.git/has"
	"code.wolfmud.org/WolfMUD.git/recordjar/decode"
	"code.wolfmud.org/WolfMUD.git/recordjar/encode"
	"code.wolfmud.org/WolfMUD.git/text/tree"
)

// Register marshaler for Action attribute.
func init() {
	internal.AddMarshaler((*Action)(nil), "action")
}

// Action implements an Attribute to display random action messages. Messages
// are specified via an OnAction Attribute. Action schedules a $action command
// to display a message everytime the event fires. The period when Action fires
// is between Action.after and Action.after+Action.jitter. An Action event can
// be cancelled by calling Action.Abort or by closing the Action.Cancel
// channel.
type Action struct {
	Attribute
	after  time.Duration
	jitter time.Duration
	due    time.Time
	event.Cancel
}

// Some interfaces we want to make sure we implement
var (
	_ has.Action = &Action{}
)

// NewAction returns a new Action attribute initialised with the passed after
// and jitter durations. The after and jitter Duration set the delay period to
// between after and after+jitter for when a Thing performs an action.
func NewAction(after time.Duration, jitter time.Duration) *Action {
	return &Action{Attribute{}, after, jitter, time.Time{}, nil}
}

// FindAction searches the attributes of the specified Thing for attributes
// that implement has.Action returning the first match it finds or a *Action
// typed nil otherwise.
func FindAction(t has.Thing) has.Action {
	return t.FindAttr((*Action)(nil)).(has.Action)
}

// Is returns true if passed attribute implements an action else false.
func (*Action) Is(a has.Attribute) bool {
	_, ok := a.(has.Action)
	return ok
}

// Found returns false if the receiver is nil otherwise true.
func (a *Action) Found() bool {
	return a != nil
}

// Unmarshal is used to turn the passed data into a new Action attribute.
func (*Action) Unmarshal(data []byte) has.Attribute {
	a := NewAction(0, 0)
	for field, data := range decode.PairList(data) {
		data := []byte(data)
		switch field {
		case "AFTER":
			a.after = decode.Duration(data)
		case "JITTER":
			a.jitter = decode.Duration(data)
		default:
			log.Printf("Action.unmarshal unknown attribute: %q: %q", field, data)
		}
	}
	return a
}

// Marshal returns a tag and []byte that represents the receiver.
func (a *Action) Marshal() (tag string, data []byte) {
	tag = "action"
	data = encode.PairList(
		map[string]string{
			"after":  string(encode.Duration(a.after)),
			"jitter": string(encode.Duration(a.jitter)),
		},
		'→',
	)
	return
}

// Dump adds attribute information to the passed tree.Node for debugging.
func (a *Action) Dump(node *tree.Node) *tree.Node {
	node = node.Append("%p %[1]T - after: %s, jitter: %s", a, a.after, a.jitter)
	dueIn := time.Until(a.due).Truncate(time.Second)
	if a.Cancel != nil && dueIn > 0 {
		node.Branch().Append("%p %[1]T - due: %s", a.Cancel, dueIn)
	} else {
		node.Branch().Append("%p %[1]T - due: expired", a.Cancel)
	}
	return node
}

// Copy returns a copy of the Action receiver. If an Action event is currently
// in-flight it will not be rescheduled automatically.
func (a *Action) Copy() has.Attribute {
	if a == nil {
		return (*Action)(nil)
	}
	na := NewAction(a.after, a.jitter)
	na.due = a.due
	return na
}

// Action schedules an action. If there is already an action event pending it
// will be cancelled and a new one queued.
func (a *Action) Action() {
	if a == nil {
		return
	}

	a.Abort()

	p := a.Parent()
	oa := FindOnAction(p)
	if oa.Found() {
		a.Cancel, a.due = event.Queue(p, "$ACTION "+oa.ActionText(), a.after, a.jitter)
	}
}

// Abort cancels any pending action events.
func (a *Action) Abort() {
	if a == nil {
		return
	}

	if a.Cancel != nil {
		close(a.Cancel)
		a.Cancel = nil
	}
}

// Pending returns true if there is an Action event pending, else false. Use
// with caution as this could introduce a race between checking the state and
// acting on it as the event could fire between the two actions.
func (a *Action) Pending() bool {
	return a.Cancel != nil
}

// Free makes sure references are nil'ed and channels closed when the Action
// attribute is freed.
func (a *Action) Free() {
	if a == nil {
		return
	}
	a.Abort()
	a.Attribute.Free()
}
