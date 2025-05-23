package testcases

import (
	"fmt"
	"testing"

	"github.com/d-protocol/pokertable"
	"github.com/google/uuid"
	"github.com/thoas/go-funk"
)

func LogJSON(t *testing.T, msg string, jsonPrinter func() (string, error)) {
	json, _ := jsonPrinter()
	fmt.Printf("\n===== [%s] =====\n%s\n", msg, json)
}

func NewDefaultTableSetting(joinPlayers ...pokertable.JoinPlayer) pokertable.TableSetting {
	return pokertable.TableSetting{
		TableID: uuid.New().String(),
		Meta: pokertable.TableMeta{
			CompetitionID:       "1005c477-84b4-4d1b-9fca-3a6ad84e0fe7",
			Rule:                pokertable.CompetitionRule_Default,
			Mode:                pokertable.CompetitionMode_CT,
			MaxDuration:         3,
			TableMaxSeatCount:   9,
			TableMinPlayerCount: 2,
			MinChipUnit:         10,
			ActionTime:          10,
		},
		Blind: pokertable.TableBlindState{
			Level:  1,
			Ante:   0,
			Dealer: 0,
			SB:     10,
			BB:     20,
		},
		JoinPlayers: joinPlayers,
	}
}

func currentPlayerMove(table *pokertable.Table) (string, []string) {
	playerID := ""
	currGamePlayerIdx := table.State.GameState.Status.CurrentPlayer
	for gamePlayerIdx, playerIdx := range table.State.GamePlayerIndexes {
		if gamePlayerIdx == currGamePlayerIdx {
			playerID = table.State.PlayerStates[playerIdx].PlayerID
			break
		}
	}
	return playerID, table.State.GameState.Players[currGamePlayerIdx].AllowedActions
}

func findPlayerID(table *pokertable.Table, position string) string {
	for _, playerIdx := range table.State.GamePlayerIndexes {
		player := table.State.PlayerStates[playerIdx]
		if funk.Contains(player.Positions, position) {
			return player.PlayerID
		}
	}
	return ""
}
