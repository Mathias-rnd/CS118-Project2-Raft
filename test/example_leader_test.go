package test

import (
    raft "raft/pkg"
    "testing"
    "time"
    "context"
)

// Tests that nodes can successfully join a cluster and elect a leader.
func TestStudentInit(t *testing.T) {
    suppressLoggers()
    config := raft.DefaultConfig()
    cluster, err := raft.CreateLocalCluster(config)
    defer cleanupCluster(cluster)
    if err != nil {
        t.Fatal(err)
    }

    // wait for a leader to be elected
    time.Sleep(time.Second * WaitPeriod)

    followers, candidates, leaders := 0, 0, 0
    for i := 0; i < config.ClusterSize; i++ {
        node := cluster[i]
        switch node.GetState() {
        case raft.FollowerState:
            followers++
        case raft.CandidateState:
            candidates++
        case raft.LeaderState:
            leaders++
        }
    }

    if followers != config.ClusterSize-1 {
        t.Errorf("follower count mismatch, expected %v, got %v", config.ClusterSize-1, followers)
    }

    if candidates != 0 {
        t.Errorf("candidate count mismatch, expected %v, got %v", 0, candidates)
    }

    if leaders != 1 {
        t.Errorf("leader count mismatch, expected %v, got %v", 1, leaders)
    }
}

// Tests that if a leader is partitioned from its followers, a
// new leader is elected.
func TestStudentNewElection(t *testing.T) {
    suppressLoggers()
    config := raft.DefaultConfig()
    config.ClusterSize = 5

    cluster, err := raft.CreateLocalCluster(config)
    defer cleanupCluster(cluster)
    if err != nil {
        t.Fatal(err)
    }

    // wait for a leader to be elected
    time.Sleep(time.Second * WaitPeriod)
    oldLeader, err := findLeader(cluster)
    if err != nil {
        t.Fatal(err)
    }

    // partition leader, triggering election
    oldTerm := oldLeader.GetCurrentTerm()
    oldLeader.NetworkPolicy.PauseWorld(true)

    // wait for new leader to be elected
    time.Sleep(time.Second * WaitPeriod)

    // unpause old leader and wait for it to become a follower
    oldLeader.NetworkPolicy.PauseWorld(false)
    time.Sleep(time.Second * WaitPeriod)

    newLeader, err := findLeader(cluster)
    if err != nil {
        t.Fatal(err)
    }

    if oldLeader.Self.Id == newLeader.Self.Id {
        t.Errorf("leader did not change")
    }

    if newLeader.GetCurrentTerm() == oldTerm {
        t.Errorf("term did not change")
    }
}

//test 1
func TestFollowerRejectsClientRequests(t *testing.T) {
    suppressLoggers()
    config := raft.DefaultConfig()
    config.ClusterSize = 3
    config.ElectionTimeout = 5 * time.Second

    cluster, _ := raft.CreateLocalCluster(config)
    defer cleanupCluster(cluster)

    time.Sleep(1 * time.Second)

    // Pick a non-leader
    leader, _ := findLeader(cluster)
    var follower *raft.Node
    for _, n := range cluster {
        if n != leader { follower = n; break }
    }

    req := raft.ClientRequest{
        ClientId:        0,
        SequenceNum:     1,
        StateMachineCmd: 1,
        Data:            []byte("hello"),
    }

    reply, _ := follower.ClientRequestCaller(context.Background(), &req)

    if reply.Status != raft.ClientStatus_NOT_LEADER {
        t.Fatalf("Expected NOT_LEADER, got %v", reply.Status)
    }
}


//test2 
func TestFollowerElectionTimeoutBecomesCandidate(t *testing.T) {
    suppressLoggers()
    config := raft.DefaultConfig()
    config.ClusterSize = 1
    config.ElectionTimeout = 400 * time.Millisecond

    cluster, _ := raft.CreateLocalCluster(config)
    defer cleanupCluster(cluster)

    time.Sleep(1 * time.Second)

    if cluster[0].GetState() != raft.LeaderState {
        t.Fatalf("Single node should become leader automatically")
    }
}

//test 3
func TestCandidateWinsElection(t *testing.T) {
    suppressLoggers()
    config := raft.DefaultConfig()
    config.ClusterSize = 3

    cluster, _ := raft.CreateLocalCluster(config)
    defer cleanupCluster(cluster)

    time.Sleep(1 * time.Second)

    leader, err := findLeader(cluster)
    if err != nil {
        t.Fatalf("No leader elected")
    }

    if leader.GetState() != raft.LeaderState {
        t.Fatalf("Expected leader, got %v", leader.GetState())
    }
}

//test 4
func TestCandidateStepsDownOnAppendEntries(t *testing.T) {
    suppressLoggers()
    config := raft.DefaultConfig()
    config.ClusterSize = 3

    cluster, _ := raft.CreateLocalCluster(config)
    defer cleanupCluster(cluster)

    time.Sleep(1 * time.Second)

    leader, _ := findLeader(cluster)

    // Force a follower into candidate state by isolating it
    var follower *raft.Node
    for _, n := range cluster {
        if n != leader { follower = n; break }
    }

    follower.NetworkPolicy.PauseWorld(true)
    time.Sleep(time.Second)

    follower.NetworkPolicy.PauseWorld(false)

    // Wait for leader heartbeat
    time.Sleep(1 * time.Second)

    if follower.GetState() != raft.FollowerState {
        t.Fatalf("Candidate must step down when receiving AppendEntries")
    }
}

//test 5 (new)
func TestSingleNodeBecomesLeader(t *testing.T) {
    suppressLoggers()
    config := raft.DefaultConfig()
    config.ClusterSize = 1
    config.ElectionTimeout = 300 * time.Millisecond

    cluster, _ := raft.CreateLocalCluster(config)
    defer cleanupCluster(cluster)

    time.Sleep(1 * time.Second)

    if cluster[0].GetState() != raft.LeaderState {
        t.Fatal("Single node should immediately become leader")
    }
}

//test 6
func TestLeaderCrashTriggersNewElection(t *testing.T) {
    suppressLoggers()
    config := raft.DefaultConfig()
    config.ClusterSize = 5

    cluster, _ := raft.CreateLocalCluster(config)
    defer cleanupCluster(cluster)

    time.Sleep(time.Second * WaitPeriod)
    
    oldLeader, err := findLeader(cluster)
    if err != nil {
        t.Fatal(err)
    }
    oldTerm := oldLeader.GetCurrentTerm()

    // Simulate leader crash
    oldLeader.GracefulExit()

    // Wait for new election
    time.Sleep(time.Second * WaitPeriod)

    // Find new leader among remaining nodes
    remainingCluster := make([]*raft.Node, 0)
    for _, node := range cluster {
        if node != oldLeader {
            remainingCluster = append(remainingCluster, node)
        }
    }

    newLeader, err := findLeader(remainingCluster)
    if err != nil {
        t.Fatal("No new leader elected after crash")
    }

    if newLeader.GetCurrentTerm() <= oldTerm {
        t.Error("New leader should have higher term")
    }
}

//test 7
func TestDuplicateRequestsAreCached(t *testing.T) {
    suppressLoggers()
    config := raft.DefaultConfig()
    cluster, _ := raft.CreateLocalCluster(config)
    defer cleanupCluster(cluster)

    time.Sleep(time.Second * WaitPeriod)

    leader, _ := findLeader(cluster)

    // Send same request twice
    req := raft.ClientRequest{
        ClientId:        1,
        SequenceNum:     1,
        StateMachineCmd: hashmachine.HashChainInit,
        Data:            []byte("test"),
    }

    reply1, _ := leader.ClientRequestCaller(context.Background(), &req)
    reply2, _ := leader.ClientRequestCaller(context.Background(), &req)

    if reply1.Status != raft.ClientStatus_OK || reply2.Status != raft.ClientStatus_OK {
        t.Error("Both requests should succeed")
    }

    // Responses should be identical (cached)
    if string(reply1.Response) != string(reply2.Response) {
        t.Error("Duplicate requests should return cached response")
    }
}
