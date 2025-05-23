package pokertable

type TablePlayerGameAction struct {
	CompetitionID    string   `json:"competition_id"`
	TableID          string   `json:"table_id"`
	GameID           string   `json:"game_id"`
	GameCount        int      `json:"game_count"`
	UpdateAt         int64    `json:"update_at"`
	PlayerID         string   `json:"player_id"`
	Action           string   `json:"action"`
	Round            string   `json:"round"`
	Chips            int64    `json:"chips"`
	Seat             int      `json:"seat"`
	Positions        []string `json:"positions"`
	Bankroll         int64    `json:"bankroll"`
	InitialStackSize int64    `json:"initial_stack_size"`
	StackSize        int64    `json:"stack_size"`
	Pot              int64    `json:"pot"`
	Wager            int64    `json:"wager"`
}

type TableSetting struct {
	TableID     string          `json:"table_id"`
	Meta        TableMeta       `json:"table_meta"`
	JoinPlayers []JoinPlayer    `json:"join_players"`
	Blind       TableBlindState `json:"blind"`
}

type JoinPlayer struct {
	PlayerID    string `json:"player_id"`
	RedeemChips int64  `json:"redeem_chips"`
	Seat        int    `json:"seat"`
}

// TableBlindState represents the blind state of a poker table
type TableBlindState struct {
	Level   int   `json:"level"`    // Current blind level, -1 represents a breaking level
	Ante    int64 `json:"ante"`     // Ante amount that each player must contribute
	Dealer  int64 `json:"dealer"`   // Dealer blind amount
	SB      int64 `json:"sb"`       // Small blind amount
	BB      int64 `json:"bb"`       // Big blind amount
	EndTime int64 `json:"end_time"` // Optional time when this blind level ends (unix timestamp)
}

// IsSet returns true if the blind state is properly configured
func (bs *TableBlindState) IsSet() bool {
	// A blind state is considered set if it has a valid level (can be -1 for breaking)
	// and the blind amounts are properly defined
	return bs != nil && bs.Level != 0 && bs.SB > 0 && bs.BB > 0
}

// IsBreaking returns true if the table is in a breaking period
// (typically between blind levels in tournaments)
func (bs *TableBlindState) IsBreaking() bool {
	return bs != nil && bs.Level == -1
}
