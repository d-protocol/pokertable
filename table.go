package pokertable

import (
	"encoding/json"

	"github.com/d-protocol/pokerlib"
)

const (
	TableStateStatus_TableGameStandby = "standby"
	TableStateStatus_TableGamePlaying = "playing"
	TableStateStatus_TableClosed      = "closed"
)

type TableEngine interface {
	PlayerPass(playerID string) error
	PlayerReady(playerID string) error
	PlayerPay(playerID string, chips int64) error
	PlayerCheck(playerID string) error
	PlayerBet(playerID string, chips int64) error
	PlayerCall(playerID string) error
	PlayerFold(playerID string) error
	PlayerAllin(playerID string) error
}

type TableMeta struct {
	ActionTime int `json:"action_time"`
}

type TableStateStatus string

type TablePlayerState struct {
	PlayerID  string   `json:"player_id"`
	Seat      int      `json:"seat"`
	Positions []string `json:"positions"`
	Bankroll  int64    `json:"bankroll"`
}

type TableState struct {
	Status           TableStateStatus       `json:"status"`
	GameState        *pokerlib.GameState    `json:"game_state"`
	PlayerStates     []*TablePlayerState    `json:"player_states"`
	GamePlayerIndexes []int                 `json:"game_player_indexes"`
}

type Table struct {
	ID    string     `json:"id"`
	Meta  TableMeta  `json:"meta"`
	State *TableState `json:"state"`
}

func (t *Table) GamePlayerIndex(playerID string) int {
	// Simplified implementation
	return 0
}

func (t *Table) GetJSON() (string, error) {
	data, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type TableSetting struct {
	// Add fields as needed
}