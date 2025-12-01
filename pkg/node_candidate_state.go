package pkg

// doCandidate implements the logic for a Raft node in the candidate state.
func (n *Node) doCandidate() stateFunction {
	n.Out("Transitioning to CANDIDATE_STATE")
	n.setState(CandidateState)

	// Increment current term and vote for self
	n.SetCurrentTerm(n.GetCurrentTerm() + 1)
	n.setVotedFor(n.Self.Id)

	timeout := randomTimeout(n.Config.ElectionTimeout)

	// Request votes from all other nodes
	fallback, electionResult := n.requestVotes(n.GetCurrentTerm())

	if fallback {
		return n.doFollower
	}

	if electionResult {
		// Won the election, become leader
		return n.doLeader
	}
	// TODO: Students should implement this method
	// Hint: perform any initial work, and then consider what a node in the
	// candidate state should do when it receives an incoming message on every
	// possible channel.
	for {
		select {
		// Case: election timeout without new leader, run again
		case <-timeout:
			return n.doCandidate

		// Case client receives heartbeat -- leader exists
		case msg := <-n.appendEntries:
			_, fallback := n.handleAppendEntries(msg)
			if fallback {
				return n.doFollower
			}

		// Case: get votes// successful election
		case candidate := <-n.requestVote:
			fallback := n.handleRequestVote(candidate)
			if fallback {
				// timeout = randomTimeout(n.Config.ElectionTimeout)
				return n.doFollower
			}

		// Case: candidate receives client request
		case clientRequestMsg := <-n.clientRequest:
			leader := n.getLeader()
			clientRequestMsg.reply <- ClientReply{
				Status:     ClientStatus_ELECTION_IN_PROGRESS,
				ClientId:   clientRequestMsg.request.ClientId,
				Response:   nil,
				LeaderHint: leader,
			}

		// case follower to shutdown state
		case shutdown := <-n.gracefulExit:
			if shutdown {
				return nil
			}
		}
	}
}

// requestVotes is called to request votes from all other nodes. It takes in a
// channel on which the result of the vote should be sent over: true for a
// successful election, false otherwise.
func (n *Node) requestVotes(currTerm uint64) (fallback, electionResult bool) {
	// TODO: Students should implement this method

	nodes := n.getPeers()
	majority := len(nodes) / 2 + 1
	votesReceived := 1 // vote for self

	if votesReceived >= majority {
		return false, true
	}
	
	replies := make(chan bool, len(nodes)-1)

	for _, node := range nodes {
		if node.Id == n.Self.Id {
			continue
		}
		go func(p *RemoteNode) {
			lastLogIdx := n.LastLogIndex()
			lastLogTerm := uint64(0)
			if lastLogIdx >= 0 {
				lastLogTerm = n.GetLog(lastLogIdx).TermId
			}

			req := &RequestVoteRequest{
				Term:         currTerm,
				Candidate:    n.Self,
				LastLogIndex: n.LastLogIndex(),
				LastLogTerm:  lastLogTerm,
			}

			reply, err := p.RequestVoteRPC(n, req)
			if err == nil && reply.Term > currTerm {
			
				n.SetCurrentTerm(reply.Term)
				n.setVotedFor("")
				replies <- false
			} else {
				replies <- (err == nil && reply.VoteGranted)
			}
        }(node)

	}

    //    Loop runs for each peer we sent a request to
    for i := 0; i < len(nodes)-1; i++ {
        // Receive result from channel
		voteGranted := <-replies
        if voteGranted {
            // If vote granted, increment count
            votesReceived++
            // Check if we have majority (more than half)
            if votesReceived >= majority { 
                return false, true // Won election!
            }
        } else if n.GetCurrentTerm() > currTerm {
            // If we discovered a higher term (fallback signal), stop election
            return true, false // Fallback to follower
        }
    }
    
    // If loop finishes without majority or fallback, election failed
    return false, false
}

// handleRequestVote handles an incoming vote request. It returns true if the caller
// should fall back to the follower state, false otherwise.
func (n *Node) handleRequestVote(msg RequestVoteMsg) (fallback bool) {

	// Candidate is behind (n has larger term), vote no
	if n.GetCurrentTerm() > msg.request.Term {
		msg.reply <- RequestVoteReply{
			Term:        n.GetCurrentTerm(),
			VoteGranted: false,
		}
		return false // Vote no
	} 
	
	// Term is higher, so we update our term and fallback to follower
	if msg.request.Term > n.GetCurrentTerm() {
		n.SetCurrentTerm(msg.request.Term)
		n.setVotedFor("")
		fallback = true
	}
	
	// check if we can vote for the candidate
	canVote := (n.GetVotedFor() == "" || n.GetVotedFor() == msg.request.Candidate.Id)

	lastLogIDx := n.LastLogIndex()
	lastLogTerm := uint64(0)
	if lastLogIDx >= 0 {
		lastLogTerm = n.GetLog(lastLogIDx).TermId
	}

	// Check if candidate's log is up to date
	LogisUpToDate := false
	if msg.request.LastLogTerm > lastLogTerm {
		LogisUpToDate = true
	} else if msg.request.LastLogTerm == lastLogTerm && msg.request.LastLogIndex >= lastLogIDx {
		LogisUpToDate = true
	} 

	// If both conditions are met, we can grant the vote
	if canVote && LogisUpToDate {
		n.setVotedFor(msg.request.Candidate.Id)
		msg.reply <- RequestVoteReply{
			Term:        n.GetCurrentTerm(),
			VoteGranted: true,
		}
	// condition is not met
	} else {
		msg.reply <- RequestVoteReply{
			Term:        n.GetCurrentTerm(),
			VoteGranted: false,
		}
	}

	return fallback
	
}
