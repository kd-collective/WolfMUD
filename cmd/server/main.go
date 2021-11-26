// Copyright 2021 Andrew 'Diddymus' Rolfe. All rights reserved.
//
// Use of this source code is governed by the license in the LICENSE file
// included with the source code.

package main

import (
	"log"
	"math/rand"
	"net"
	"time"

	"code.wolfmud.org/WolfMUD.git/client"
	"code.wolfmud.org/WolfMUD.git/core"
	"code.wolfmud.org/WolfMUD.git/world"
)

func main() {

	rand.Seed(time.Now().UnixNano())

	// Stop the world while we are building it
	core.BWL.Lock()
	core.RegisterCommandHandlers()
	world.Load()
	core.BWL.Unlock()

	addr, _ := net.ResolveTCPAddr("tcp", ":4001")
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		log.Printf("Error setting up listener: %s", err)
		return
	}

	log.Printf("Accepting connections on: %s", addr)
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			log.Printf("Error accepting connection: %s", err)
			continue
		}
		c := client.New(conn)
		go c.Play()
	}
}
