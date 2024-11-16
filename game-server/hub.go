// Copyright (c) 2013 The Gorilla WebSocket Authors. All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
//   Redistributions of source code must retain the above copyright notice, this
//   list of conditions and the following disclaimer.
//
//   Redistributions in binary form must reproduce the above copyright notice,
//   this list of conditions and the following disclaimer in the documentation
//   and/or other materials provided with the distribution.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
// ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
// WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
// FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
// SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
// CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
// OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package main

import (
	"encoding/json"
	"fmt"
	"time"
)

// Hub maintains the set of active clients and broadcasts messages to the
// clients.
type Hub struct {
	// Registered clients.
	clients map[*Client]bool

	// Inbound messages from the clients.
	broadcast chan *Update

	// Register requests from the clients.
	register chan *Client

	// Unregister requests from clients.
	unregister chan *Client

	// Rooms
	rooms []*Room
}

func newHub() *Hub {
	return &Hub{
		broadcast:  make(chan *Update),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		rooms:      []*Room{},
	}
}

func (h *Hub) run() {
	for {
		select {

		case client := <-h.register:
			h.clients[client] = true

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}

		case update := <-h.broadcast:

			// skip updates if they do not have a room set
			if update.Room == "" {
				continue
			}

			// skip updates if they do not have a user id set
			if update.ID == "" {
				continue
			}

			// do room stuff
			h.handleUpdate(update)

			for client := range h.clients {
				// skip this client if this is meant for a specific user and it is not this user
				if update.TargetUserID != "" && update.TargetUserID != client.ID {
					continue
				}

				// skip this client if it is not in the target room
				if client.Room != update.Room {
					continue
				}

				// do not send an update back to the same client it originated from unless ret=true
				if client.ID == update.ID && !update.Ret {
					continue
				}

				fmt.Println("Sending update: Module(", update.Module+"),  TargetUser("+client.ID+")")

				// encode update into a byte array
				jsonData, err := json.Marshal(update)
				if err != nil {
					fmt.Println(err)
					continue
				}

				// write the update to the client
				select {
				case client.send <- jsonData:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}

func (h *Hub) handleUpdate(update *Update) {
	fmt.Println("Received update: Module(", update.Module+"),  From("+update.ID+")")

	switch Module(update.Module) {
	case JoinEditorModule:
		h.handleJoinEditorModule(update)
	case HostEditorModule:
		h.handleHostEditorModule(update)
		update.TargetUserID = update.ID
	case RequestEditorModule:
		h.handleRequestEditorModule(update)
	case ResponseEditorModule:
		h.handleResponseEditorModule(update)
	case RequestRoomModule:
		h.handleRequestRoomModule(update)
	case EditorModule:
		h.handleEditorModule(update)
	}
}

func (h *Hub) handleJoinEditorModule(update *Update) {
	room, found := h.findRoom(update.Room)
	if !found {
		h.setError(update, RoomNotFoundErrorMessage, update.ID)
		return
	}

	room.MembersID = append(room.MembersID, update.ID)

	update.Module = string(JoinSuccessModule)
	update.MemberIDList = room.MembersID
	update.HostID = room.HostID
}

func (h *Hub) handleHostEditorModule(update *Update) {
	if update.Room == "" {
		h.setError(update, RoomNotFoundErrorMessage, update.ID)
		return
	}

	if _, exists := h.findRoom(update.Room); exists {
		h.setError(update, RoomExistsErrorMessage, update.ID)
		return
	}

	h.rooms = append(h.rooms, &Room{
		Name:      update.Room,
		HostID:    update.ID,
		MembersID: []string{update.ID},
	})
	update.Module = string(HostSuccessModule)
	update.TargetUserID = update.ID
}

func (h *Hub) handleRequestEditorModule(update *Update) {
	if update.Room == "" {
		h.setError(update, RoomNotFoundErrorMessage, update.ID)
		return
	}

	room, found := h.findRoom(update.Room)
	if !found {
		h.setError(update, RoomNotFoundErrorMessage, update.ID)
		return
	}

	update.TargetUserID = room.HostID
}

func (h *Hub) handleResponseEditorModule(update *Update) {
	if update.Room == "" {
		h.setError(update, RoomNotFoundErrorMessage, update.ID)
		return
	}

	_, found := h.findRoom(update.Room)
	if !found {
		h.setError(update, RoomNotFoundErrorMessage, update.ID)
		return
	}
}

func (h *Hub) handleRequestRoomModule(update *Update) {
	room, found := h.findRoom(update.Room)
	if !found {
		h.setError(update, RoomNotFoundErrorMessage, update.ID)
		return
	}
	update.Module = string(ResponseRoomModule)
	update.MemberIDList = room.MembersID
	update.HostID = room.HostID
}

func (h *Hub) handleEditorModule(update *Update) {
	if update.Editor == nil {
		update.Editor = &LevelEditorUpdate{}
	}
	update.Editor.Timestamp = time.Now().UnixMilli()
}

func (h *Hub) findRoom(roomName string) (*Room, bool) {
	for i := range h.rooms {
		if h.rooms[i].Name == roomName {
			return h.rooms[i], true
		}
	}
	return nil, false
}

func (h *Hub) setError(update *Update, errorMessage ErrorMessage, targetID string) {
	update.ErrorMessage = string(errorMessage)
	update.TargetUserID = targetID
}