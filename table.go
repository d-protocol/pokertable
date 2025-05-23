package pokertable

import (
	"encoding/json"

	"github.com/d-protocol/pokerlib"
)

const (
	TableStateStatus_TableGameStandby = "standby"
	TableStateStatus_TableGamePlaying = "playing"
	TableStateStatus_TableClosed      = "closed"

	// Add missing status constants
	TableStateStatus_TableCreated     = "created"
	TableStateStatus_TablePausing     = "pausing"
	TableStateStatus_TableBalancing   = "balancing"
	TableStateStatus_TableGameOpened  = "game_opened"
	TableStateStatus_TableGameSettled = "game_settled"
)

// type TableEngine interface {
// 	PlayerPass(playerID string) error
// 	PlayerReady(playerID string) error
// 	PlayerPay(playerID string, chips int64) error
// 	PlayerCheck(playerID string) error
// 	PlayerBet(playerID string, chips int64) error
// 	PlayerCall(playerID string) error
// 	PlayerFold(playerID string) error
// 	PlayerAllin(playerID string) error
// }

type TableMeta struct {
	CompetitionID       string `json:"competition_id"`
	Rule                string `json:"rule"`
	Mode                string `json:"mode"`
	MaxDuration         int    `json:"max_duration"`
	TableMaxSeatCount   int    `json:"table_max_seat_count"`
	TableMinPlayerCount int    `json:"table_min_player_count"`
	MinChipUnit         int    `json:"min_chip_unit"`
	ActionTime          int    `json:"action_time"`
}

type TableStateStatus string

type TablePlayerState struct {
	PlayerID       string                    `json:"player_id"`
	Seat           int                       `json:"seat"`
	Positions      []string                  `json:"positions"`
	Bankroll       int64                     `json:"bankroll"`
	IsIn           bool                      `json:"is_in"`           // Player has joined the table
	IsParticipated bool                      `json:"is_participated"` // Player is participating in the current game
	GameStatistics TablePlayerGameStatistics `json:"game_statistics"` // Player's game statistics
}

type TableState struct {
	Status               TableStateStatus       `json:"status"`
	GameState            *pokerlib.GameState    `json:"game_state"`
	PlayerStates         []*TablePlayerState    `json:"player_states"`
	GamePlayerIndexes    []int                  `json:"game_player_indexes"`
	GameCount            int                    `json:"game_count"`
	StartAt              int64                  `json:"start_at"`
	BlindState           *TableBlindState       `json:"blind_state"`
	CurrentDealerSeat    int                    `json:"current_dealer_seat"`
	CurrentSBSeat        int                    `json:"current_sb_seat"`
	CurrentBBSeat        int                    `json:"current_bb_seat"`
	SeatMap              map[int]int            `json:"seat_map"`
	NextBBOrderPlayerIDs []string               `json:"next_bb_order_player_ids"`
	LastPlayerGameAction *TablePlayerGameAction `json:"last_player_game_action"`
	CurrentActionEndAt   int64                  `json:"current_action_end_at"`
	GameBlindState       *TableBlindState       `json:"game_blind_state"`
}

type Table struct {
	ID           string      `json:"id"`
	Meta         TableMeta   `json:"meta"`
	State        *TableState `json:"state"`
	UpdateAt     int64       `json:"update_at"`     // Last update timestamp
	UpdateSerial int         `json:"update_serial"` // Incremental update counter
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

// FindPlayerIdx finds player index by playerID
func (t *Table) FindPlayerIdx(playerID string) int {
	for i, playerState := range t.State.PlayerStates {
		if playerState.PlayerID == playerID {
			return i
		}
	}
	return UnsetValue
}

// FindGamePlayerIdx finds game player index by playerID
func (t *Table) FindGamePlayerIdx(playerID string) int {
	for gamePlayerIdx, playerIdx := range t.State.GamePlayerIndexes {
		if t.State.PlayerStates[playerIdx].PlayerID == playerID {
			return gamePlayerIdx
		}
	}
	return UnsetValue
}

// FindPlayerIndexFromGamePlayerIndex converts game player index to table player index
func (t *Table) FindPlayerIndexFromGamePlayerIndex(gamePlayerIdx int) int {
	if gamePlayerIdx < 0 || gamePlayerIdx >= len(t.State.GamePlayerIndexes) {
		return UnsetValue
	}
	return t.State.GamePlayerIndexes[gamePlayerIdx]
}

// PlayerSeatMap returns a map of player IDs to seat numbers
func (t *Table) PlayerSeatMap() map[string]int {
	playerSeatMap := make(map[string]int)
	for _, player := range t.State.PlayerStates {
		playerSeatMap[player.PlayerID] = player.Seat
	}
	return playerSeatMap
}

// AlivePlayers returns a list of players who have chips
func (t *Table) AlivePlayers() []*TablePlayerState {
	alivePlayers := make([]*TablePlayerState, 0)
	for _, player := range t.State.PlayerStates {
		if player.Bankroll > 0 {
			alivePlayers = append(alivePlayers, player)
		}
	}
	return alivePlayers
}

// Clone creates a deep copy of the table
func (t *Table) Clone() (*Table, error) {
	jsonData, err := t.GetJSON()
	if err != nil {
		return nil, err
	}

	newTable := &Table{}
	err = json.Unmarshal([]byte(jsonData), newTable)
	if err != nil {
		return nil, err
	}

	return newTable, nil
}

// ShouldPause determines if the table should be paused
func (t *Table) ShouldPause() bool {
	// A simple implementation - could be enhanced based on actual logic
	return t.State.BlindState != nil && t.State.BlindState.Level == -1
}
