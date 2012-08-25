// Copyright 2012 Andrew 'Diddymus' Rolfe. All rights resolved.
//
// Use of this source code is governed by the license in the LICENSE file
// included with the source code.

// Package command implements the representation and state of a command that is
// being processed - or a string of commands.
package command

import (
	"strings"
	"wolfmud.org/entities/thing"
	"wolfmud.org/utils/messaging"
)

// Interface should be implemented by anything that wants to process/react
// to commands. These commands are usually from players and mobiles but also
// commonly from other objects. For example a lever when pulled may issue an
// 'OPEN DOOR' command to get the door associated with the lever to open.
//
// The Process method when called should return true if the command was
// processed by the Thing implementing Process. Note that handled means the
// command was dealt with by a Thing. The outcome may be a success or a failure
// - but it WAS still handled.
//
// TODO: Beef up description when examples available.
//
// TODO: Need to document locking
//
// TODO: Document command.NEW vs NEW
type Interface interface {
	Process(*Command) (handled bool)
}

// Command represents the state of the command currently being processed.
// Command is also used to pass around locking information as the command is
// processed.
type Command struct {
	Issuer        thing.Interface   // What is issuing the command
	Verb          string            // 1st word (verb): GET, DROP, EXAMINE etc
	Nouns         []string          // 2nd...nth words
	Target        string            // Alias for 2nd word - normally the verb's target
	Locks         []thing.Interface // Locks we want to hold
	locksModified bool              // Locks modified since last LocksModified() call?
	response      responseBuffer
	broadcast     broadcastBuffer
}

// responseBuffer stores buffered messages send by Respond.
type responseBuffer struct {
	format []string
	any    []interface{}
}

// reset clears the responseBuffer so it can be reused.
func (rb *responseBuffer) reset() {
	rb.format, rb.any = nil, nil
}

// broadcastBuffer stores buffered messages send by Broadcast.
type broadcastBuffer struct {
	responseBuffer
	omit []thing.Interface
}

// reset clears the broadcastBuffer so it can be reused.
func (bb *broadcastBuffer) reset() {
	bb.responseBuffer.reset()
	bb.omit = nil
}

// New creates a new Command instance. The input string is assigned via a call
// to command.New() which documents the details.
func New(issuer thing.Interface, input string) *Command {
	cmd := Command{Issuer: issuer}
	cmd.New(input)
	return &cmd
}

// New assigns a new input string to an existing command instance created using
// New. The input string is broken into words using whitespace as the separator.
// The first word is usually the verb - get, drop, examine - and the second word
// the target noun to act on - get ball, drop ball, examine ball. As this is a
// common case the second word cam also referenced via the alias Target.
//
// Assigning a new input string is useful when you want to issue multiple
// commands but keep the same locks and buffers. For example assume you have
// some items and you drop them all in one go by issuing 'DROP ALL'. Internally
// we can get the aliases for each item in the inventory and loop over them
// issuing:
//
//	cmd.New("DROP "+o.Aliases()[0])
//
func (c *Command) New(input string) {
	words := strings.Split(strings.ToUpper(input), ` `)
	c.Verb, c.Nouns = words[0], words[1:]
	if len(words) > 1 {
		c.Target = words[1]
	} else {
		c.Target = ""
	}
}

// Flush processes the buffered messages sent using Respond and Broadcast.
func (c *Command) Flush() {
	if len(c.response.format) > 0 {
		if r, ok := c.Issuer.(messaging.Responder); ok {
			format := strings.Join(c.response.format, "\n")
			r.Respond(format, c.response.any...)
		}
		c.response.reset()
	}

	if len(c.broadcast.format) > 0 {
		if b, ok := c.Issuer.(messaging.Broadcaster); ok {
			format := strings.Join(c.broadcast.format, "\n")
			b.Broadcast(c.broadcast.omit, format, c.broadcast.any...)
		}
		c.broadcast.reset()
	}
}

// Respond implements the responder Interface. It is a quick shorthand for
// responding to the Thing that is issuing the command, with buffering, without
// having to do any additional bookkeeping.
func (c *Command) Respond(format string, any ...interface{}) {
	if _, ok := c.Issuer.(messaging.Responder); ok {
		c.response.format = append(c.response.format, format)
		c.response.any = append(c.response.any, any...)
	}
}

// Broadcast implements the broadcaster Interface. It is a quick shorthand
// for broadcasting to the Thing's location that is issuing the command, with
// buffering, without having to do any additional book keeping.
func (c *Command) Broadcast(omit []thing.Interface, format string, any ...interface{}) {
	if _, ok := c.Issuer.(messaging.Broadcaster); ok {

		// Add omitted things - but not duplicates!
	OMIT:
		for _, o1 := range omit {
			for _, o2 := range c.broadcast.omit {
				if o1.IsAlso(o2) {
					continue OMIT
				}
			}
			c.broadcast.omit = append(c.broadcast.omit, o1)
		}

		c.broadcast.format = append(c.broadcast.format, format)
		c.broadcast.any = append(c.broadcast.any, any...)
	}
}

// CanLock checks if the command has the thing in it's locks list. This only
// determines if the thing is in the Locks slice - not if it is or is not
// actually locked. This is because we may have just added the lock and have
// not actually tried locking or relocking yet.
func (c *Command) CanLock(thing thing.Interface) bool {
	for _, l := range c.Locks {
		if thing.IsAlso(l) {
			return true
		}
	}
	return false
}

// LocksModified returns true if the Locks slice has been modified since the
// Command was created or since the last call of LocksModified.
//
// NOTE: Calling this function will also reset the check to false.
func (c *Command) LocksModified() (modified bool) {
	modified, c.locksModified = c.locksModified, false
	return modified
}

// AddLock takes a reference to a thing and adds it to the Locks slice in the
// correct position. Locks should always be acquired in unique Id sequence
// lowest to highest to avoid deadlocks. By using this method the Locks property
// can easily be iterated via a range and in the correct sequence required.
//
// NOTE: We cannot add the same Lock twice otherwise we would deadlock ourself
// when locking - currently we silently drop duplicate locks.
//
// This routine is a little cute and avoids doing any 'real' sorting to keep the
// elements in unique ID sequence. We add our lock to our slice. If we have one
// element only it's what we just added so we bail.
//
// If we have multiple elements we have the appended element on the end and need
// to check where it goes, shift the trailing element up by one then write our
// new element in:
//
//	3 7 9 4 <- append new element to end
//	3 7 9 4 <- correct place: 4 goes between 3 and 7
//	3 7 7 9 <- shift 7,9 up one overwriting our appended element
//	3 4 7 9 <- we now write our new element into our 'hole'
//
// What if we can't find an element with a unique Id greater than the one we are
// inserting?
//
//	3 7 9 10 <- append new element to end
//	3 7 9 10 <- correct place: 10 goes after 9, not insert point found
//	3 7 9 10 <- No shifting is done, appended element not over-written
//	3 4 7 10 <- new element already in correct place, nothing else to do
//
// This function could be more efficient with large numbers of elements by
// using a binary search to find the insertion point for the new element.
// However this would make the code more complex and we don't expect to handle
// huge numbers of locks with this function.
func (c *Command) AddLock(t thing.Interface) {

	if t == nil || c.CanLock(t) {
		return
	}

	c.locksModified = true
	c.Locks = append(c.Locks, t)

	if l := len(c.Locks); l > 1 {
		uid := t.UniqueId()
		for i := 0; i < l; i++ {
			if uid > c.Locks[i].UniqueId() {
				copy(c.Locks[i+1:l], c.Locks[i:l-1])
				c.Locks[i] = t
				break
			}
		}
	}
}
