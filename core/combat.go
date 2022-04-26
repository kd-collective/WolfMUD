// Copyright 2022 Andrew 'Diddymus' Rolfe. All rights reserved.
//
// Use of this source code is governed by the license in the LICENSE file
// included with the source code.

package core

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"code.wolfmud.org/WolfMUD.git/text"
)

var roundDuration = (3 * time.Second).Nanoseconds()

func (s *state) Hit() {

	if len(s.word) == 0 {
		s.Msg(s.actor, text.Info, "You go to hit... someone?")
		return
	}

	damage := 2 + rand.Int63n(2+1)
	damageTxt := strconv.FormatInt(damage, 10)
	where := s.actor.Ref[Where]
	notify := len(where.Who) < cfg.crowdSize

	uids := Match(s.word, where)
	uid := uids[0]
	what := where.Who[uid]
	if what == nil {
		what = where.In[uid]
	}

	switch {
	case what == nil:
		s.Msg(s.actor, text.Bad, "You see no '", uid, "' to hit.")
	case s.actor == what:
		s.Msg(s.actor, text.Good, "You give yourself a slap. Awake now?")
		s.Msg(where, text.Info, s.actor.As[UName], " slaps themself.")
	case where.As[VetoCombat] != "":
		s.Msg(s.actor, text.Bad, where.As[VetoCombat])
	case !notify:
		s.Msg(s.actor, text.Bad, "It's too crowded to start a fight.")
	case what.Int[HealthMaximum] == 0:
		s.Msg(s.actor, text.Bad, "You cannot kill ", what.As[Name], ".")
	case what.Int[HealthCurrent] <= damage:

		// Helper to center text within 80 columns
		center := func(text string) string {
			pad := (80 - len(text)) / 2
			return strings.ReplaceAll(strings.Repeat("␠", pad)+text, " ", "␠")
		}

		s.Msg(s.actor, text.Good, "You kill ", what.As[TheName], " (", damageTxt, ").")
		s.Msg(what, text.Bad, s.actor.As[UTheName],
			" kills you (", damageTxt, ").",
			text.Cyan,
			"\n",
			"\n", center(" :==[ Rest In Peace ]==:"),
			"\n",
			"\n", center(what.As[Name]),
			"\n", center("Slain By"),
			"\n", center(s.actor.As[Name]),
			text.Good,
			"\n\nYou must know people in high places, you are to be given another chance...\n",
		)

		s.Msg(where, text.Info,
			"You see ", s.actor.As[TheName], " kill ", what.As[Name], ".")

		// Create and place corpse
		c := createCorpse(what)
		where.In[c.As[UID]] = c
		c.Schedule(Cleanup)

		// Remove original
		if what.Is&Player == 0 {
			what.Int[HealthCurrent] = what.Int[HealthMaximum]
			what.Junk()
		} else {
			what.Int[HealthCurrent] = 1
			delete(where.Who, what.As[UID])
			start := WorldStart[rand.Intn(len(WorldStart))]
			what.Ref[Where] = start
			start.Who[what.As[UID]] = what
			s.subparseFor(what, "$POOF")
		}
		Prompt[what.As[PromptStyle]](what)

	default:
		what.Int[HealthCurrent] -= damage
		if what.Event[Health] == nil && what.Int[HealthCurrent] < what.Int[HealthMaximum] {
			what.Schedule(Health)
		}
		Prompt[what.As[PromptStyle]](what)

		s.Msg(s.actor, text.Good, "You hit ", what.As[TheName], " (", damageTxt, ").")
		s.Msg(what, text.Bad, s.actor.As[UTheName], " hits you (", damageTxt, ").")
		s.Msg(where, text.Info,
			"You see ", s.actor.As[Name], " hit ", what.As[Name], ".")

		if what.Int[HealthCurrent] < 4 {
			s.MsgAppend(s.actor, text.Good, " ", what.As[UTheName], " looks nearly dead.")
			s.MsgAppend(what, text.Bad, " You are almost dead.")
			s.MsgAppend(where, text.Info, " ", what.As[UTheName], " is almost dead.")
		}

		locs := radius(1, s.actor.Ref[Where])
		for _, where := range locs[1] {
			if l := len(where.Who); 0 < l && l < cfg.crowdSize {
				s.Msg(where, text.Info, "You hear fighting nearby.")
			}
		}
	}
}

func createCorpse(t *Thing) *Thing {
	c := NewThing()
	c.As[Name] = "a corpse of " + t.As[Name]
	c.As[UName] = "A corpse of " + t.As[Name]
	c.As[TheName] = "the corpse of " + t.As[Name]
	c.As[UTheName] = "The corpse of " + t.As[Name]
	c.As[Description] = t.As[Description]
	c.Any[Alias] = append(c.Any[Alias], t.Any[Alias]...)
	c.Any[Qualifier] = append(c.Any[Qualifier], t.Any[Qualifier]...)
	c.Ref[Where] = t.Ref[Where]
	c.Ref[Where].In[c.As[UID]] = t
	c.Int[CleanupAfter] = time.Duration(60 * time.Second).Nanoseconds()
	c.As[OnCleanup] = c.As[UTheName] + " turns to dust."

	// Replace original UID alias with "CORPSE" (new UID was added by NewThing)
	for x, alias := range c.Any[Alias] {
		if alias == t.As[UID] {
			c.Any[Alias][x] = "CORPSE"
		}
	}

	return c
}

func (s *state) Combat() {

	what := s.actor.Ref[Opponent]
	where := s.actor.Ref[Where]

	if what == nil || where != what.Ref[Where] {
		s.stopCombat(s.actor, nil)
		s.Msg(s.actor, text.Info, "\nYou stop fighting, your opponent disappeared...")
		return
	}

	attacker, defender := s.actor, what
	if rand.Int63n(100+1) < 50 {
		attacker, defender = defender, attacker
	}

	damage := 2 + rand.Int63n(2+1)
	damageText := fmt.Sprintf(" doing %d damage.", damage)
	defender.Int[HealthCurrent] -= damage

	s.Msg(s.actor, "\n") // Actor needs manually moving off of prompt
	s.MsgAppend(attacker, text.Good, "You hit ", defender.As[TheName], damageText)
	s.MsgAppend(defender, text.Bad, attacker.As[UTheName], " hits you", damageText)
	s.Msg(where, text.Info, attacker.As[UTheName], " hits ", defender.As[Name], ".")

	// defender not killed, do health bookkeeping and go another round
	if defender.Int[HealthCurrent] > 0 {
		Prompt[defender.As[PromptStyle]](defender)
		if defender.Event[Health] == nil {
			defender.Schedule(Health)
		}
		s.actor.Int[CombatAfter] = roundDuration
		s.actor.Schedule(Combat)
		return
	}
	s.Msg(attacker, "You kill ", defender.As[Name], "!")
	s.Msg(defender, attacker.As[UTheName], " kills you!")
	s.Msg(where, attacker.As[UTheName], " kills ", defender.As[Name], "!")

	// Stop everyone attacking defender and notify them, as they receive a
	// specific message they won't get the message to the location.
	for _, uid := range defender.Any[Opponents] {
		who := where.Who[uid]
		if who != nil && who != attacker {
			s.Msg(who, text.Info, attacker.As[UTheName], " kills ", defender.As[Name], "!")
			s.Msg(who, text.Info, "You stop fighting ", defender.As[Name], ".")
		}
		if who == nil {
			who = where.In[uid]
		}
		s.stopCombat(who, defender)
	}
	s.stopCombat(defender, nil)

	// Create and place corpse
	c := createCorpse(defender)
	where.In[c.As[UID]] = c
	c.Schedule(Cleanup)

	// Remove defender from location
	delete(where.Who, defender.As[UID])

	// If not a player junk for a reset
	if defender.Is&Player == 0 {
		defender.Junk()
		return
	}

	// Place player back into the world
	start := WorldStart[rand.Intn(len(WorldStart))]
	defender.Int[HealthCurrent] = 1
	defender.Ref[Where] = start
	start.Who[defender.As[UID]] = defender

	if s.actor == defender {
		s.subparse("$POOF")
	} else {
		s.subparseFor(defender, "$POOF")
	}
}

func (s *state) stopCombat(who, what *Thing) {
	if what == nil {
		who.Cancel(Combat)
		who.Schedule(Action)
		delete(who.Ref, Opponent)
		delete(who.Any, Opponents)
	} else {
		who.Any[Opponents], _ = remainder(who.Any[Opponents], []string{what.As[UID]})
		if len(who.Any[Opponents]) == 0 {
			who.Schedule(Action)
			who.Cancel(Combat)
			delete(who.Ref, Opponent)
			delete(who.Any, Opponents)
		}
	}
}
