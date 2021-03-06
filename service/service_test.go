package service

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"github.com/nullstyle/go-xdr/xdr3"
	"github.com/spacemeshos/merkle-tree"
	"github.com/spacemeshos/poet/prover"
	"github.com/spacemeshos/poet/signal"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"strconv"
	"testing"
	"time"
)

type MockBroadcaster struct {
	receivedMessages chan []byte
}

func (b *MockBroadcaster) BroadcastProof(msg []byte, roundID string, members [][]byte) error {
	b.receivedMessages <- msg
	return nil
}

type challenge struct {
	data  []byte
	round *round
}

// TestService_Recovery test Service recovery functionality by tracking 3 distinct rounds.
// The scenario proceeds as follows:
// 	- Create a new service instance.
//  - Submit challenges to open round (0).
//  - Wait for round to start executing, and a new round to be open.
//  - Submit challenges to open round (1).
//  - Wait a bit for round 0 execution to proceed.
//  - Shutdown service.
//  - Create a new service instance.
//  - Submit challenges to open round (1)
//  - Wait a bit for round 1 execution to proceed.
//  - Submit challenges to open round (2).
//  - Verify that new service instance broadcast 3 distinct rounds proofs, by the expected order.
func TestService_Recovery(t *testing.T) {
	req := require.New(t)
	sig := signal.NewSignal()
	broadcaster := &MockBroadcaster{receivedMessages: make(chan []byte)}
	cfg := &Config{N: 17, InitialRoundDuration: 1 * time.Second}
	tempdir, _ := ioutil.TempDir("", "poet-test")

	// Create a new service instance.
	s, err := NewService(sig, cfg, tempdir)
	req.NoError(err)
	err = s.Start(broadcaster)
	req.NoError(err)

	// Track the service rounds.
	numRounds := 3
	rounds := make([]*round, numRounds)

	// Generate 4 groups of random challenges.
	challengeGroupSize := 40
	challengeGroups := make([][]challenge, 4)
	for i := 0; i < 4; i++ {
		challengeGroup := make([]challenge, challengeGroupSize)
		for i := 0; i < challengeGroupSize; i++ {
			challengeGroup[i] = challenge{data: make([]byte, 32)}
			_, err := rand.Read(challengeGroup[i].data)
			req.NoError(err)
		}
		challengeGroups[i] = challengeGroup
	}

	submittedChallenges := make(map[int][]challenge)
	submitChallenges := func(roundIndex int, groupIndex int) {
		challengesGroup := challengeGroups[groupIndex]
		for i := 0; i < len(challengeGroups[groupIndex]); i++ {
			round, err := s.Submit(challengesGroup[i].data)
			req.NoError(err)
			req.Equal(s.openRound.ID, round.ID)

			// Verify that all submissions returned the same round instance.
			if rounds[roundIndex] == nil {
				rounds[roundIndex] = round
			} else {
				req.Equal(rounds[roundIndex], round)
			}
		}

		// Track the submitted challenges per-round for later validation.
		submittedChallenges[roundIndex] = append(submittedChallenges[roundIndex], challengesGroup...)
	}

	// Submit challenges to open round (0).
	submitChallenges(0, 0)

	// Verify that round is still open.
	req.Equal(rounds[0].ID, s.openRound.ID)

	// Wait for round 0 to start executing.
	select {
	case <-rounds[0].executionStartedChan:
	case err := <-s.errChan:
		req.Fail(err.Error())
	}

	// Verify that round iteration proceeds: a new round opened, previous round is executing.
	req.Contains(s.executingRounds, rounds[0].ID)
	rounds[1] = s.openRound

	// Submit challenges to open round (1).
	submitChallenges(1, 1)

	// Wait a bit for round 0 execution to proceed.
	time.Sleep(1 * time.Second)

	// Request shutdown.
	sig.RequestShutdown()

	// Verify shutdown error is received.
	select {
	case err := <-s.errChan:
		req.EqualError(err, fmt.Sprintf("round %v execution error: %v", rounds[0].ID, prover.ErrShutdownRequested.Error()))
	case <-rounds[0].executionEndedChan:
		req.Fail("round execution ended instead of shutting down")
	}

	time.Sleep(1 * time.Second)

	// Verify service state. should have no open or executing rounds.
	req.Equal((*round)(nil), s.openRound)
	req.Equal(len(s.executingRounds), 0)

	// Create a new service instance.
	sig = signal.NewSignal()
	s, err = NewService(sig, cfg, tempdir)
	req.NoError(err)

	err = s.Start(broadcaster)
	req.NoError(err)
	time.Sleep(500 * time.Millisecond)

	// Service instance should recover 2 rounds: round 0 in executing state, and round 1 in open state.
	prevServiceRounds := rounds
	req.Equal(1, len(s.executingRounds))
	_, ok := s.executingRounds[prevServiceRounds[0].ID]
	req.True(ok)
	req.Equal(s.openRound.ID, prevServiceRounds[1].ID)

	// Track rounds from the new service instance.
	rounds = make([]*round, numRounds)
	rounds[0] = s.executingRounds[prevServiceRounds[0].ID]
	rounds[1] = s.openRound

	// Submit challenges to open round (1).
	submitChallenges(1, 2)

	// Wait for round 1 to start executing.
	select {
	case <-rounds[1].executionStartedChan:
	case err := <-s.errChan:
		req.Fail(err.Error())
	}

	// Verify that round iteration proceeds: a new round opened, previous round is executing.
	req.Contains(s.executingRounds, rounds[1].ID)

	// Submit challenges to open round (2).
	submitChallenges(2, 3)

	// Verify that new service instance broadcast 3 distinct rounds proofs, by the expected order.
	for i := 0; i < numRounds; i++ {
		proofMsg := PoetProofMessage{}
		select {
		case <-time.After(10 * time.Second):
			req.Fail("proof message wasn't sent")
		case msg := <-broadcaster.receivedMessages:
			_, err := xdr.Unmarshal(bytes.NewReader(msg), &proofMsg)
			req.NoError(err)
		}

		req.Equal(rounds[i].ID, proofMsg.RoundID)

		// Verify the submitted challenges.
		req.Len(proofMsg.Members, len(submittedChallenges[i]))
		for _, ch := range submittedChallenges[i] {
			req.True(contains(proofMsg.Members, ch.data))
		}

		// Verify round statement.
		mtree, err := merkle.NewTree()
		req.NoError(err)
		for _, m := range proofMsg.Members {
			req.NoError(mtree.AddLeaf(m))
		}

		proof, err := rounds[i].proof(false)
		req.NoError(err)
		req.Equal(mtree.Root(), proof.Statement)
	}
}

func contains(list [][]byte, item []byte) bool {
	for _, listItem := range list {
		if bytes.Equal(listItem, item) {
			return true
		}
	}

	return false
}

func TestNewService(t *testing.T) {
	req := require.New(t)
	tempdir, _ := ioutil.TempDir("", "poet-test")

	cfg := new(Config)
	cfg.N = 17
	cfg.InitialRoundDuration = 1 * time.Second

	s, err := NewService(signal.NewSignal(), cfg, tempdir)
	req.NoError(err)

	proofBroadcaster := &MockBroadcaster{receivedMessages: make(chan []byte)}
	err = s.Start(proofBroadcaster)
	req.NoError(err)

	challengesCount := 8
	challenges := make([]challenge, challengesCount)
	info, err := s.Info()
	req.NoError(err)

	// Generate random challenges.
	for i := 0; i < len(challenges); i++ {
		challenges[i] = challenge{data: make([]byte, 32)}
		_, err := rand.Read(challenges[i].data)
		req.NoError(err)
	}

	// Submit challenges.
	for i := 0; i < len(challenges); i++ {
		round, err := s.Submit(challenges[i].data)
		req.NoError(err)
		req.Equal(info.OpenRoundID, round.ID)
		challenges[i].round = round

		// Verify that all submissions returned the same round instance.
		if i > 0 {
			req.Equal(challenges[i].round, challenges[i-1].round)
		}
	}

	// Verify that round is still open.
	info, err = s.Info()
	req.NoError(err)
	req.Equal(info.OpenRoundID, info.OpenRoundID)

	// Wait for round to start execution.
	select {
	case <-challenges[0].round.executionStartedChan:
	case err := <-s.errChan:
		req.Fail(err.Error())
	}

	// Verify that round iteration proceeded.
	prevInfo := info
	info, err = s.Info()
	req.NoError(err)
	prevIndex, err := strconv.Atoi(prevInfo.OpenRoundID)
	req.NoError(err)
	req.Equal(fmt.Sprintf("%d", prevIndex+1), info.OpenRoundID)
	req.Contains(info.ExecutingRoundsIds, prevInfo.OpenRoundID)

	// Wait for end of execution.
	select {
	case <-challenges[0].round.executionEndedChan:
	case err := <-s.errChan:
		req.Fail(err.Error())
	}

	// Wait for proof message broadcast.
	select {
	case msg := <-proofBroadcaster.receivedMessages:
		poetProof := PoetProofMessage{}
		_, err := xdr.Unmarshal(bytes.NewReader(msg), &poetProof)
		req.NoError(err)
	case <-time.After(100 * time.Millisecond):
		req.Fail("proof message wasn't sent")
	}
}

func genChallenges(num int) ([][]byte, error) {
	ch := make([][]byte, num)
	for i := 0; i < num; i++ {
		ch[i] = make([]byte, 32)
		_, err := rand.Read(ch[i])
		if err != nil {
			return nil, err
		}
	}

	return ch, nil
}
