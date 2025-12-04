*****************************
RAFT PROJECT 3
Names: Yvette, Yvonne, Mathias

*****************************


# About
Implementation Overview: 
Our program implements a Raft consensus protocol. We implemented the core logic of the algorithm across three files representing the three different node states: follower, candidate, and leader. For each state, our program defines node behavior for four channels: RaftNode.appendEntries, RaftNode.requestVote, and RaftNode.clientRequest, RaftNode.gracefulExit. These cases were handled in a select statement within an infinite loop, where different node behaviors triggered different channels, causing nodes to transition to different states or update internal logs/states accordingly. This change may be triggered by things like network partitions, heartbeat timeouts, client requests, etc. We implemented election logic to enable a node's transition to a leader state, using various checks to uphold the leader completeness property and a mandatory majority consensus. 


Tell us about your implementation and tests here

1 - TestFollowerRejectsClientRequests

Purpose:
Verifies that a follower never processes client requests, and instead returns the correct NOT_LEADER response.

What it tests:
Correct follower behavior under the Raft spec
Follower redirection logic
Ensures follower does not accidentally commit or try to run state machine commands


2 - TestFollowerElectionTimeoutBecomesCandidate

Purpose:
Ensures that a follower correctly starts a new election after an election timeout.

What it tests:
Election timeout logic
Follower to Candidate to Leader transitions
Covers follower timeout branch in doFollower
Covers candidate self-election in doCandidate


3 - TestCandidateWinsElection

Purpose:
Checks that a valid election occurs and that exactly one leader is elected.

What it tests:
Raft election mechanics
Candidate broadcast of RequestVote RPCs
Majority vote counting


4 - TestCandidateStepsDownOnAppendEntries

Purpose:
Verifies that a candidate immediately steps down upon receiving AppendEntries from a valid leader.

What it tests:
Candidate to Follower fallback on higher term AppendEntries
Coverage of handleAppendEntries branch that forces fallback
Ensures leader heartbeats override an ongoing election


5 - TestSingleNodeBecomesLeader

Purpose:
Validates that a single-node cluster immediately elects itself as leader without delay.

What it tests:
Edge case of single-node cluster
Immediate leader election with no competition
Self-voting mechanism in requestVotes
Majority calculation with cluster size of 1
Covers the simplest case of leader election


6 - TestLeaderCrashTriggersNewElection

Purpose:
Ensures that when a leader crashes, the remaining nodes detect the failure and elect a new leader.

What it tests:
Leader failure detection via election timeout
New leader election after leader crash
Term increment in new election
Graceful exit handling
Covers follower timeout when leader is unavailable


7 - TestDuplicateRequestsAreCached

Purpose:
Verifies that the system properly caches and returns identical responses for duplicate client requests.

What it tests:
Client request caching mechanism
Duplicate detection using ClientId and SequenceNum
Cache lookup in processLogEntry



Handouts URL https://docs.google.com/document/d/18wI6L_46tsacsWvTpOBBoVAhrY-0Tzyctb3XIM6IbfY/edit?usp=sharing