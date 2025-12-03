package pkg

import (
	// "context"
	// "sort"
	"strconv"
	"strings"
	"time"
)

// doLeader implements the logic for a Raft node in the leader state.
func (n *Node) doLeader() stateFunction {
	n.Out("Transitioning to LeaderState")
	n.setState(LeaderState)

	// TODO: Students should implement this method
	// Hint: perform any initial work, and then consider what a node in the
	// leader state should do when it receives an incoming message on every
	// possible channel.

	n.setLeader(n.Self)

	n.LeaderMutex.Lock() // added
	lastLogIndex := n.LastLogIndex()
    for _, node := range n.getPeers() {
        n.nextIndex[node.Id] = lastLogIndex + 1
        n.matchIndex[node.Id] = 0
    }
	n.LeaderMutex.Unlock() // added

    noopEntry := LogEntry{ // added
        Index:   n.LastLogIndex() + 1,
        TermId:  n.GetCurrentTerm(),
        Type:    CommandType_NOOP,
        Command: 0,
        Data:    nil,
        CacheId: "",
    }
    n.StoreLog(&noopEntry) // added
	n.matchIndex[n.Self.Id] = noopEntry.Index


	fallback := n.sendHeartbeats()
    if fallback {
        return n.doFollower
    }

	ticker := time.NewTicker(n.Config.HeartbeatTimeout)
    defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fallback := n.sendHeartbeats()
			if fallback {
				return n.doFollower
			}
		
		// Channel 1 append entries
		// Another leader might exist
		case msg := <-n.appendEntries:
			_, fallback := n.handleAppendEntries(msg)
			// Check if leader term is outdated and step down if so
			if fallback {
				// Update term and step down
				return n.doFollower
			}

		// Channel 2 request vote 
		// leader should shut down candidate??
		case candidate := <-n.requestVote:
			// check candidates term and step down if leader is outdated
			// otherwise shut down candidate 
			fallback := n.handleRequestVote(candidate)
			if fallback {
				return n.doFollower
			}
			
		// Channel 3 clientRequest
		// Request from client should be logged and propagated to followers(?)
		case clientRequestMsg := <-n.clientRequest:
			// Initialize log entry

			if clientRequestMsg.request.StateMachineCmd == 0 && len(clientRequestMsg.request.Data) == 0 {
				entry := LogEntry{
					Index:   n.LastLogIndex() + 1,
					TermId:  n.GetCurrentTerm(),
					Type:    CommandType_CLIENT_REGISTRATION,
					Command: 0,
					Data:    nil,
					CacheId: "",
				}
				n.StoreLog(&entry)
				
				// Reply immediately with clientID = log index
				clientRequestMsg.reply <- ClientReply{
					Status:     ClientStatus_OK,
					ClientId:   entry.Index,  // ← ClientID is the log index!
					Response:   nil,
					LeaderHint: n.Self,
				}

			} else {

				cacheID := CreateCacheID(clientRequestMsg.request.ClientId, clientRequestMsg.request.SequenceNum)
				entry := &LogEntry{
					Index:     n.LastLogIndex() + 1,
					TermId:    n.GetCurrentTerm(),
					Type:      CommandType_STATE_MACHINE_COMMAND,
					Command:   clientRequestMsg.request.StateMachineCmd,
					Data:	   clientRequestMsg.request.Data,
					CacheId:   cacheID,
				}

				n.requestsMutex.Lock()
				n.requestsByCacheID[cacheID] = append(n.requestsByCacheID[cacheID], clientRequestMsg.reply)
				n.requestsMutex.Unlock()
				
				n.StoreLog(entry)
			}
			// commented out
			// // Append entry to leader log
			// n.StoreLog(entry)

			// // Register client
			// // Lock while changes are in progress
			// n.requestsMutex.Lock()
			// // Append cacheID/channel to requests map for later response access to channel 
			// n.requestsByCacheID[entry.CacheId] = append(
			// 	n.requestsByCacheID[entry.CacheId],
			// 	clientRequestMsg.reply,
			// )
			// // Unlock after append is complete
			// n.requestsMutex.Unlock()

			// // Propagate log entry to followers
			// n.sendHeartbeats() 


		// Channel 4: gracefulExit
		// exit leader loop?
		case shutdown := <-n.gracefulExit:
			if shutdown {
				return nil
			}
		}

	}
}



// sendHeartbeats is used by the leader to send out heartbeats to each of
// the other nodes. It returns true if the leader should fall back to the
// follower state. (This happens if we discover that we are in an old term.)
//
// If another node isn't up-to-date, then the leader should attempt to
// update them, and, if an index has made it to a quorum of nodes, commit
// up to that index. Once committed to that index, the replicated state
// machine should be given the new log entries via processLogEntry.
func (n *Node) sendHeartbeats() (fallback bool) {
	// TODO: Students should implement this method

	if n.GetState() != LeaderState {
		return true
	}

	currTerm := n.GetCurrentTerm()
	nodes := n.getPeers()
	
	// lastLogIndex := n.LastLogIndex() // what we send??

	for _, peer := range nodes {
		// skip ourselves
		if peer.Id == n.Self.Id {
			continue
		}

		go func(p *RemoteNode) {
			n.LeaderMutex.Lock()
			nextIdx := n.nextIndex[p.Id]
			n.LeaderMutex.Unlock()

			prevLogIdx := nextIdx - 1
			prevLogTerm := uint64(0)
			
			// Check that previous log index also matches
			if prevLogIdx > 0 {
				entry := n.GetLog(prevLogIdx)
				if entry != nil {
					prevLogTerm = entry.TermId
				}
			}

			var entries []*LogEntry
			lastLogIdx := n.LastLogIndex()
			
			// Store p's missing entries in entries arr
            if nextIdx <= lastLogIdx {
                entries = make([]*LogEntry, 0, lastLogIdx-nextIdx+1)
                for i := nextIdx; i <= lastLogIdx; i++ {
                    if entry := n.GetLog(i); entry != nil {
                        entries = append(entries, entry)
                    }
                }
            }
			
            leaderCommit := n.CommitIndex.Load()

			req := &AppendEntriesRequest{
                Term:         currTerm,
                Leader:       n.Self,
                PrevLogIndex: prevLogIdx,
                PrevLogTerm:  prevLogTerm,
                Entries:      entries,
                LeaderCommit: leaderCommit,
            }

			reply, err := p.AppendEntriesRPC(n, req)
				if err != nil {
					return
				}

			// node has higher term, we step down to follower
			if reply.Term > currTerm {
				n.SetCurrentTerm(reply.Term)
				n.setState(FollowerState)
				return
			}

			if reply.Success {
				// update peer match index and next match index
				newMatchIndex := prevLogIdx + uint64(len(entries))
				// compare new match index to curr p match index
				n.LeaderMutex.Lock()
				if newMatchIndex > n.matchIndex[p.Id] {
					n.matchIndex[p.Id] = newMatchIndex
					n.nextIndex[p.Id] = newMatchIndex + 1
				}
				// n.LeaderMutex.Unlock()

				// check if majority success commit
				
				for N := n.LastLogIndex(); N > n.CommitIndex.Load(); N-- {
					if n.GetLog(N).TermId != currTerm {
						continue
					}

					count := 1 // include leader
					// Count how many other nodes have committed entry

					// n.LeaderMutex.Lock()
					for _, otherPeer := range nodes {
						// Skip self, already counted in initialization
						if otherPeer.Id == n.Self.Id {
							continue
						}
						// Check for peers
						if n.matchIndex[otherPeer.Id] >= N {
							count++
						}
					}
					// n.LeaderMutex.Unlock()
					

					// Check if quorum (first majority will be highest value of N with a majority)
					if count > len(nodes)/2 {
						// Quorum !!! we can commit
						n.CommitIndex.Store(N)
						start := n.LastApplied.Load() + 1
						for i := start; i <= N; i++ {
							n.processLogEntry(i)
							n.LastApplied.Store(i)
						}
						break
					}	
				}
				n.LeaderMutex.Unlock()
			} else {
				n.LeaderMutex.Lock()
				if next := n.nextIndex[p.Id]; next > 1 {
					n.nextIndex[p.Id] = next - 1
				}
				n.LeaderMutex.Unlock()
			}
		}(peer)
	}

	return n.GetState() != LeaderState
}

// processLogEntry applies a single log entry to the finite state machine. It is
// called once a log entry has been replicated to a majority and committed by
// the leader. Once the entry has been applied, the leader responds to the client
// with the result, and also caches the response.
func (n *Node) processLogEntry(logIndex uint64) (fallback bool) {
	fallback = false
	entry := n.GetLog(logIndex)
	n.Out("Processing log index: %v, entry: %v", logIndex, entry)

	status := ClientStatus_OK
	var response []byte
	var err error
	var clientId uint64

	switch entry.Type {
	case CommandType_NOOP:
		return
	case CommandType_CLIENT_REGISTRATION:
		clientId = logIndex
	case CommandType_STATE_MACHINE_COMMAND:
		if clientId, err = strconv.ParseUint(strings.Split(entry.GetCacheId(), "-")[0], 10, 64); err != nil {
			panic(err)
		}
		if resp, ok := n.GetCachedReply(entry.GetCacheId()); ok {
			status = resp.GetStatus()
			response = resp.GetResponse()
		} else {
			response, err = n.StateMachine.ApplyCommand(entry.Command, entry.Data)
			if err != nil {
				status = ClientStatus_REQ_FAILED
				response = []byte(err.Error())
			}
		}
	}

	// Construct reply
	reply := ClientReply{
		Status:     status,
		ClientId:   clientId,
		Response:   response,
		LeaderHint: &RemoteNode{Addr: n.Self.Addr, Id: n.Self.Id},
	}

	// Send reply to client
	n.requestsMutex.Lock()
	defer n.requestsMutex.Unlock()
	// Add reply to cache
	if entry.CacheId != "" {
		if err = n.CacheClientReply(entry.CacheId, reply); err != nil {
			panic(err)
		}
	}
	if replies, exists := n.requestsByCacheID[entry.CacheId]; exists {
		for _, ch := range replies {
			ch <- reply
		}
		delete(n.requestsByCacheID, entry.CacheId)
	}

	return
}
