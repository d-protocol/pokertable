package pokertable

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/d-protocol/pokertable/open_game_manager"
	"github.com/d-protocol/pokertable/seat_manager"
	"github.com/d-protocol/syncsaga"
	"github.com/d-protocol/timebank"
)

var (
	ErrTableNoEmptySeats                       = errors.New("table: no empty seats available")
	ErrTableInvalidCreateSetting               = errors.New("table: invalid create table setting")
	ErrTablePlayerNotFound                     = errors.New("table: player not found")
	ErrTablePlayerInvalidGameAction            = errors.New("table: player invalid game action")
	ErrTablePlayerInvalidAction                = errors.New("table: player invalid action")
	ErrTablePlayerSeatUnavailable              = errors.New("table: player seat unavailable")
	ErrTableOpenGameFailed                     = errors.New("table: failed to open game")
	ErrTableOpenGameFailedInBlindBreakingLevel = errors.New("table: unable to open game when blind level is breaking")
)

type TableEngineOpt func(*tableEngine)

type TableEngine interface {
	// Events
	OnTableUpdated(fn func(table *Table))
	OnTableErrorUpdated(fn func(table *Table, err error))
	OnTableStateUpdated(fn func(event string, table *Table))
	OnTablePlayerStateUpdated(fn func(competitionID, tableID string, playerState *TablePlayerState))
	OnTablePlayerReserved(fn func(competitionID, tableID string, playerState *TablePlayerState))
	OnGamePlayerActionUpdated(fn func(gameAction TablePlayerGameAction))
	OnAutoGameOpenEnd(fn func(competitionID, tableID string))
	OnReadyOpenFirstTableGame(fn func(competitionID, tableID string, gameCount int, playerStates []*TablePlayerState))

	// Other Actions
	ReleaseTable() error

	// Table Actions
	GetTable() *Table                                                                             // Get table
	GetGame() Game                                                                                // Get game engine
	CreateTable(tableSetting TableSetting) (*Table, error)                                        // Create table
	PauseTable() error                                                                            // Pause table
	CloseTable() error                                                                            // Close table
	StartTableGame() error                                                                        // Start table game
	UpdateBlind(level int, ante, dealer, sb, bb int64)                                            // Update current blind info
	SetUpTableGame(gameCount int, participants map[string]int)                                    // Setup game
	UpdateTablePlayers(joinPlayers []JoinPlayer, leavePlayerIDs []string) (map[string]int, error) // Update table players

	// Player Table Actions
	PlayerReserve(joinPlayer JoinPlayer) error     // Player reserve seat
	PlayerJoin(playerID string) error              // Player join table
	PlayerSettlementFinish(playerID string) error  // Player settlement complete
	PlayerRedeemChips(joinPlayer JoinPlayer) error // Player redeem chips
	PlayersLeave(playerIDs []string) error         // Players leave table

	// Player Game Actions
	PlayerExtendActionDeadline(playerID string, duration int) (int64, error) // Extend player action deadline
	PlayerReady(playerID string) error                                       // Player ready
	PlayerPay(playerID string, chips int64) error                            // Player pay
	PlayerBet(playerID string, chips int64) error                            // Player bet
	PlayerRaise(playerID string, chipLevel int64) error                      // Player raise
	PlayerCall(playerID string) error                                        // Player call
	PlayerAllin(playerID string) error                                       // Player all-in
	PlayerCheck(playerID string) error                                       // Player check
	PlayerFold(playerID string) error                                        // Player fold
	PlayerPass(playerID string) error                                        // Player pass
}

type tableEngine struct {
	lock                      sync.Mutex
	options                   *TableEngineOptions
	table                     *Table
	game                      Game
	gameBackend               GameBackend
	rg                        *syncsaga.ReadyGroup
	tbForOpenGame             *timebank.TimeBank
	sm                        seat_manager.SeatManager
	ogm                       open_game_manager.OpenGameManager
	onTableUpdated            func(table *Table)
	onTableErrorUpdated       func(table *Table, err error)
	onTableStateUpdated       func(event string, table *Table)
	onTablePlayerStateUpdated func(competitionID, tableID string, playerState *TablePlayerState)
	onTablePlayerReserved     func(competitionID, tableID string, playerState *TablePlayerState)
	onGamePlayerActionUpdated func(gameAction TablePlayerGameAction)
	onAutoGameOpenEnd         func(competitionID, tableID string)
	onReadyOpenFirstTableGame func(competitionID, tableID string, gameCount int, playerStates []*TablePlayerState)
	isReleased                bool
}

func NewTableEngine(options *TableEngineOptions, opts ...TableEngineOpt) TableEngine {
	callbacks := NewTableEngineCallbacks()
	te := &tableEngine{
		options:                   options,
		rg:                        syncsaga.NewReadyGroup(),
		tbForOpenGame:             timebank.NewTimeBank(),
		onTableUpdated:            callbacks.OnTableUpdated,
		onTableErrorUpdated:       callbacks.OnTableErrorUpdated,
		onTableStateUpdated:       callbacks.OnTableStateUpdated,
		onTablePlayerStateUpdated: callbacks.OnTablePlayerStateUpdated,
		onTablePlayerReserved:     callbacks.OnTablePlayerReserved,
		onGamePlayerActionUpdated: callbacks.OnGamePlayerActionUpdated,
		onAutoGameOpenEnd:         callbacks.OnAutoGameOpenEnd,
		onReadyOpenFirstTableGame: callbacks.OnReadyOpenFirstTableGame,
		isReleased:                false,
	}

	for _, opt := range opts {
		opt(te)
	}

	return te
}

func WithGameBackend(gb GameBackend) TableEngineOpt {
	return func(te *tableEngine) {
		te.gameBackend = gb
	}
}

func (te *tableEngine) OnTableUpdated(fn func(*Table)) {
	te.onTableUpdated = fn
}

func (te *tableEngine) OnTableErrorUpdated(fn func(*Table, error)) {
	te.onTableErrorUpdated = fn
}

func (te *tableEngine) OnTableStateUpdated(fn func(string, *Table)) {
	te.onTableStateUpdated = fn
}

func (te *tableEngine) OnTablePlayerStateUpdated(fn func(string, string, *TablePlayerState)) {
	te.onTablePlayerStateUpdated = fn
}

func (te *tableEngine) OnTablePlayerReserved(fn func(competitionID, tableID string, playerState *TablePlayerState)) {
	te.onTablePlayerReserved = fn
}

func (te *tableEngine) OnGamePlayerActionUpdated(fn func(TablePlayerGameAction)) {
	te.onGamePlayerActionUpdated = fn
}

func (te *tableEngine) OnAutoGameOpenEnd(fn func(competitionID, tableID string)) {
	te.onAutoGameOpenEnd = fn
}

func (te *tableEngine) OnReadyOpenFirstTableGame(fn func(competitionID, tableID string, gameCount int, playerStates []*TablePlayerState)) {
	te.onReadyOpenFirstTableGame = fn
}

func (te *tableEngine) ReleaseTable() error {
	te.isReleased = true
	return nil
}

func (te *tableEngine) GetTable() *Table {
	return te.table
}

func (te *tableEngine) GetGame() Game {
	return te.game
}

func (te *tableEngine) CreateTable(tableSetting TableSetting) (*Table, error) {
	// validate tableSetting
	if len(tableSetting.JoinPlayers) > tableSetting.Meta.TableMaxSeatCount {
		return nil, ErrTableInvalidCreateSetting
	}

	// init seat manager
	te.sm = seat_manager.NewSeatManager(tableSetting.Meta.TableMaxSeatCount, tableSetting.Meta.Rule)

	// init open game manager
	te.ogm = open_game_manager.NewOpenGameManager(open_game_manager.OpenGameOption{
		Timeout: 2,
		OnOpenGameReady: func(state open_game_manager.OpenGameState) {
			if len(state.Participants) <= 1 {
				return
			}

			// 大於一個人，開局
			if err := te.tableGameOpen(); err != nil {
				te.emitErrorEvent("OnOpenGameReady#tableGameOpen", "", err)
			}
		},
	})

	// create table instance
	table := &Table{
		ID: tableSetting.TableID,
	}

	// configure meta
	table.Meta = tableSetting.Meta

	// configure state
	status := TableStateStatus(TableStateStatus_TableCreated)
	if tableSetting.Blind.Level == -1 {
		status = TableStateStatus(TableStateStatus_TablePausing)
	}
	state := TableState{
		GameCount:            0,
		StartAt:              UnsetValue,
		BlindState:           &tableSetting.Blind,
		CurrentDealerSeat:    UnsetValue,
		CurrentSBSeat:        UnsetValue,
		CurrentBBSeat:        UnsetValue,
		SeatMap:              NewDefaultSeatMap(tableSetting.Meta.TableMaxSeatCount),
		PlayerStates:         make([]*TablePlayerState, 0),
		GamePlayerIndexes:    make([]int, 0),
		Status:               status,
		NextBBOrderPlayerIDs: make([]string, 0),
	}
	table.State = &state
	te.table = table

	te.emitEvent("CreateTable", "")
	te.emitTableStateEvent(TableStateEvent_Created)

	// handle auto join players
	if len(tableSetting.JoinPlayers) > 0 {
		if err := te.batchAddPlayers(tableSetting.JoinPlayers); err != nil {
			return nil, err
		}

		// status should be table-balancing when mtt auto create new table & join players (except for 中場休息)
		if table.Meta.Mode == CompetitionMode_MTT && table.State.Status != TableStateStatus_TablePausing {
			table.State.Status = TableStateStatus_TableBalancing
			te.emitTableStateEvent(TableStateEvent_StatusUpdated)
		}

		te.emitEvent("CreateTable -> Auto Add Players", "")
	}

	return te.table, nil
}

/*
PauseTable pauses the table
  - Use case: External pausing of auto game opening
*/
func (te *tableEngine) PauseTable() error {
	te.table.State.Status = TableStateStatus_TablePausing
	te.emitTableStateEvent(TableStateEvent_StatusUpdated)
	return nil
}

/*
CloseTable closes the table
  - Use cases: Forced close, auto close due to timeout, normal close
*/
func (te *tableEngine) CloseTable() error {
	te.table.State.Status = TableStateStatus_TableClosed
	te.ReleaseTable()

	te.emitEvent("CloseTable", "")
	te.emitTableStateEvent(TableStateEvent_StatusUpdated)
	return nil
}

func (te *tableEngine) StartTableGame() error {
	if te.table.State.StartAt != UnsetValue {
		fmt.Println("[DEBUG#StartTableGame] Table game is already started.")
		return nil
	}

	// Update start time
	te.table.State.StartAt = time.Now().Unix()
	te.emitEvent("StartTableGame", "")

	// Start the game
	te.emitReadyOpenFirstTableGame(te.table.State.GameCount, te.table.State.PlayerStates)
	return nil

}

func (te *tableEngine) UpdateBlind(level int, ante, dealer, sb, bb int64) {
	te.table.State.BlindState.Level = level
	te.table.State.BlindState.Ante = ante
	te.table.State.BlindState.Dealer = dealer
	te.table.State.BlindState.SB = sb
	te.table.State.BlindState.BB = bb
}

/*
SetUpTableGame sets up a specific hand
  - Use cases:
    1. After game start, preparing the first hand
    2. At the end of each hand, in the Continue phase, preparing the next hand
*/
func (te *tableEngine) SetUpTableGame(gameCount int, participants map[string]int) {
	te.ogm.Setup(gameCount, participants)
}

/*
UpdateTablePlayers updates the number of players at the table
  - Use case: After each hand ends
*/
func (te *tableEngine) UpdateTablePlayers(joinPlayers []JoinPlayer, leavePlayerIDs []string) (map[string]int, error) {
	te.lock.Lock()
	defer te.lock.Unlock()

	// remove players
	if len(leavePlayerIDs) > 0 {
		if err := te.batchRemovePlayers(leavePlayerIDs); err != nil {
			return nil, err
		}
	}

	// add players
	joinPlayerIDs := make([]string, 0)
	if len(joinPlayers) > 0 {
		for _, joinPlayer := range joinPlayers {
			joinPlayerIDs = append(joinPlayerIDs, joinPlayer.PlayerID)
		}

		if err := te.batchAddPlayers(joinPlayers); err != nil {
			return nil, err
		}
	}

	te.emitEvent("UpdateTablePlayers", fmt.Sprintf("joinPlayers: %s, leavePlayerIDs: %s", strings.Join(joinPlayerIDs, ","), strings.Join(leavePlayerIDs, ",")))

	return te.table.PlayerSeatMap(), nil
}

/*
PlayerReserve player confirms seat
  - Use case: Player brings chips to register or rebuys
*/
func (te *tableEngine) PlayerReserve(joinPlayer JoinPlayer) error {
	te.lock.Lock()
	defer te.lock.Unlock()

	// find player index in PlayerStates
	targetPlayerIdx := te.table.FindPlayerIdx(joinPlayer.PlayerID)

	if targetPlayerIdx == UnsetValue {
		if len(te.table.State.PlayerStates) == te.table.Meta.TableMaxSeatCount {
			return ErrTableNoEmptySeats
		}

		// BuyIn
		if err := te.batchAddPlayers([]JoinPlayer{joinPlayer}); err != nil {
			return err
		}
	} else {
		// ReBuy
		playerState := te.table.State.PlayerStates[targetPlayerIdx]
		playerState.Bankroll += joinPlayer.RedeemChips
		if err := te.sm.UpdatePlayerHasChips(playerState.PlayerID, true); err != nil {
			return err
		}

		te.emitTablePlayerStateEvent(playerState)
		te.emitTablePlayerReservedEvent(playerState)
	}

	te.emitEvent("PlayerReserve", joinPlayer.PlayerID)

	return nil
}

/*
PlayerJoin player joins the table
  - Use case: When a player has confirmed a seat and joins the table
*/
func (te *tableEngine) PlayerJoin(playerID string) error {
	playerIdx := te.table.FindPlayerIdx(playerID)
	if playerIdx == UnsetValue {
		return ErrTablePlayerNotFound
	}

	if te.table.State.PlayerStates[playerIdx].Seat == UnsetValue {
		return ErrTablePlayerInvalidAction
	}

	if te.table.State.PlayerStates[playerIdx].IsIn {
		return nil
	}

	te.table.State.PlayerStates[playerIdx].IsIn = true

	// If ReadyGroup is set and player is not ready, mark as ready
	if isReady, exist := te.rg.GetParticipantStates()[int64(playerIdx)]; exist && !isReady {
		te.rg.Ready(int64(playerIdx))
	}

	// Update seat manager
	if err := te.sm.JoinPlayers([]string{playerID}); err != nil {
		return err
	}

	te.emitEvent("PlayerJoin", playerID)
	return nil
}

/*
PlayerSettlementFinish player settlement completed
  - Use case: Player has watched the settlement animation
*/
func (te *tableEngine) PlayerSettlementFinish(playerID string) error {
	playerIdx := te.table.FindPlayerIdx(playerID)
	if playerIdx == UnsetValue {
		return ErrTablePlayerNotFound
	}

	if !te.table.State.PlayerStates[playerIdx].IsIn {
		return ErrTablePlayerInvalidAction
	}

	te.ogm.Ready(playerID)

	return nil
}

/*
PlayerRedeemChips buy-in additional chips
  - Use case: Rebuy
*/
func (te *tableEngine) PlayerRedeemChips(joinPlayer JoinPlayer) error {
	// find player index in PlayerStates
	playerIdx := te.table.FindPlayerIdx(joinPlayer.PlayerID)
	if playerIdx == UnsetValue {
		return ErrTablePlayerNotFound
	}

	playerState := te.table.State.PlayerStates[playerIdx]
	playerState.Bankroll += joinPlayer.RedeemChips

	te.emitEvent("PlayerRedeemChips", joinPlayer.PlayerID)
	te.emitTablePlayerStateEvent(playerState)
	return nil
}

/*
PlayersLeave players leave the table
  - Use cases:
  - CT: leaving table (player has chips)
  - CT: giving up rebuy (player has no chips)
  - CT: eliminated after stopping buy-in
*/
func (te *tableEngine) PlayersLeave(playerIDs []string) error {
	te.lock.Lock()
	defer te.lock.Unlock()

	if err := te.batchRemovePlayers(playerIDs); err != nil {
		return err
	}

	te.emitEvent("PlayersLeave", strings.Join(playerIDs, ","))
	te.emitTableStateEvent(TableStateEvent_PlayersLeave)

	return nil
}

/*
PlayerExtendActionDeadline extends the player's action deadline
  - Use case: When player action timer starts
*/
func (te *tableEngine) PlayerExtendActionDeadline(playerID string, duration int) (int64, error) {
	endAt := time.Unix(te.table.State.CurrentActionEndAt, 0)
	currentActionEndAt := endAt.Add(time.Duration(duration) * time.Second).Unix()
	te.table.State.CurrentActionEndAt = currentActionEndAt
	te.emitEvent("PlayerExtendActionDeadline", "")
	return currentActionEndAt, nil
}

func (te *tableEngine) PlayerReady(playerID string) error {
	te.lock.Lock()
	defer te.lock.Unlock()

	gamePlayerIdx := te.table.FindGamePlayerIdx(playerID)
	if err := te.validateGameMove(gamePlayerIdx); err != nil {
		return err
	}

	playerIdx := te.table.FindPlayerIndexFromGamePlayerIndex(gamePlayerIdx)
	if playerIdx == UnsetValue {
		return ErrGamePlayerNotFound
	}

	gs, err := te.game.Ready(gamePlayerIdx)
	if err == nil {
		te.table.State.LastPlayerGameAction = te.createPlayerGameAction(playerID, playerIdx, "ready", 0, gs.GetPlayer(gamePlayerIdx))
	}

	return err
}

func (te *tableEngine) PlayerPay(playerID string, chips int64) error {
	te.lock.Lock()
	defer te.lock.Unlock()

	gamePlayerIdx := te.table.FindGamePlayerIdx(playerID)
	if err := te.validateGameMove(gamePlayerIdx); err != nil {
		return err
	}

	playerIdx := te.table.FindPlayerIndexFromGamePlayerIndex(gamePlayerIdx)
	if playerIdx == UnsetValue {
		return ErrGamePlayerNotFound
	}

	gs, err := te.game.Pay(gamePlayerIdx, chips)
	if err == nil {
		te.table.State.LastPlayerGameAction = te.createPlayerGameAction(playerID, playerIdx, "pay", chips, gs.GetPlayer(gamePlayerIdx))
	}

	return err
}

func (te *tableEngine) PlayerBet(playerID string, chips int64) error {
	te.lock.Lock()
	defer te.lock.Unlock()

	gamePlayerIdx := te.table.FindGamePlayerIdx(playerID)
	if err := te.validateGameMove(gamePlayerIdx); err != nil {
		return err
	}

	playerIdx := te.table.FindPlayerIndexFromGamePlayerIndex(gamePlayerIdx)
	if playerIdx == UnsetValue {
		return ErrGamePlayerNotFound
	}

	gs, err := te.game.Bet(gamePlayerIdx, chips)
	if err == nil {
		te.table.State.LastPlayerGameAction = te.createPlayerGameAction(playerID, playerIdx, WagerAction_Bet, chips, gs.GetPlayer(gamePlayerIdx))
		te.emitGamePlayerActionEvent(*te.table.State.LastPlayerGameAction)

		playerState := te.table.State.PlayerStates[playerIdx]
		playerState.GameStatistics.ActionTimes++
		if te.game.GetGameState().Status.CurrentRaiser == gamePlayerIdx {
			playerState.GameStatistics.RaiseTimes++
		}

		if playerState.GameStatistics.IsVPIPChance {
			playerState.GameStatistics.IsVPIP = true
		}

		if playerState.GameStatistics.IsCBetChance {
			playerState.GameStatistics.IsCBet = true
		}
	}

	return err
}

func (te *tableEngine) PlayerRaise(playerID string, chipLevel int64) error {
	te.lock.Lock()
	defer te.lock.Unlock()

	gamePlayerIdx := te.table.FindGamePlayerIdx(playerID)
	if err := te.validateGameMove(gamePlayerIdx); err != nil {
		return err
	}

	playerIdx := te.table.FindPlayerIndexFromGamePlayerIndex(gamePlayerIdx)
	if playerIdx == UnsetValue {
		return ErrGamePlayerNotFound
	}

	gs, err := te.game.Raise(gamePlayerIdx, chipLevel)
	if err == nil {
		playerState := te.table.State.PlayerStates[playerIdx]
		te.table.State.LastPlayerGameAction = te.createPlayerGameAction(playerID, playerIdx, WagerAction_Raise, chipLevel, gs.GetPlayer(gamePlayerIdx))
		te.emitGamePlayerActionEvent(*te.table.State.LastPlayerGameAction)

		playerState.GameStatistics.ActionTimes++
		playerState.GameStatistics.RaiseTimes++

		if playerState.GameStatistics.IsVPIPChance {
			playerState.GameStatistics.IsVPIP = true
		}

		if playerState.GameStatistics.IsPFRChance {
			playerState.GameStatistics.IsPFR = true
		}

		if playerState.GameStatistics.IsATSChance {
			playerState.GameStatistics.IsATS = true
		}

		te.refreshThreeBet(playerState, playerIdx)

		if playerState.GameStatistics.IsCheckRaiseChance {
			playerState.GameStatistics.IsCheckRaise = true
		}

		if playerState.GameStatistics.IsCBetChance {
			playerState.GameStatistics.IsCBet = true
		}
	}

	return err
}

func (te *tableEngine) PlayerCall(playerID string) error {
	te.lock.Lock()
	defer te.lock.Unlock()

	gamePlayerIdx := te.table.FindGamePlayerIdx(playerID)
	if err := te.validateGameMove(gamePlayerIdx); err != nil {
		return err
	}

	playerIdx := te.table.FindPlayerIndexFromGamePlayerIndex(gamePlayerIdx)
	if playerIdx == UnsetValue {
		return ErrGamePlayerNotFound
	}

	wager := int64(0)
	if te.table.State.GameState != nil && gamePlayerIdx < len(te.table.State.GameState.Players) {
		wager = te.table.State.GameState.Status.CurrentWager - te.table.State.GameState.GetPlayer(gamePlayerIdx).Wager
	}

	gs, err := te.game.Call(gamePlayerIdx)
	if err == nil {
		te.table.State.LastPlayerGameAction = te.createPlayerGameAction(playerID, playerIdx, WagerAction_Call, wager, gs.GetPlayer(gamePlayerIdx))
		te.emitGamePlayerActionEvent(*te.table.State.LastPlayerGameAction)

		playerState := te.table.State.PlayerStates[playerIdx]
		playerState.GameStatistics.ActionTimes++
		playerState.GameStatistics.CallTimes++

		if playerState.GameStatistics.IsVPIPChance {
			playerState.GameStatistics.IsVPIP = true
		}
	}

	return err
}

func (te *tableEngine) PlayerAllin(playerID string) error {
	te.lock.Lock()
	defer te.lock.Unlock()

	gamePlayerIdx := te.table.FindGamePlayerIdx(playerID)
	if err := te.validateGameMove(gamePlayerIdx); err != nil {
		return err
	}

	playerIdx := te.table.FindPlayerIndexFromGamePlayerIndex(gamePlayerIdx)
	if playerIdx == UnsetValue {
		return ErrGamePlayerNotFound
	}

	wager := int64(0)
	if te.table.State.GameState != nil && gamePlayerIdx < len(te.table.State.GameState.Players) {
		wager = te.table.State.GameState.GetPlayer(gamePlayerIdx).StackSize
	}

	gs, err := te.game.Allin(gamePlayerIdx)
	if err == nil {
		te.table.State.LastPlayerGameAction = te.createPlayerGameAction(playerID, playerIdx, WagerAction_AllIn, wager, gs.GetPlayer(gamePlayerIdx))
		te.emitGamePlayerActionEvent(*te.table.State.LastPlayerGameAction)

		playerState := te.table.State.PlayerStates[playerIdx]
		playerState.GameStatistics.ActionTimes++
		if te.game.GetGameState().Status.CurrentRaiser == gamePlayerIdx {
			playerState.GameStatistics.RaiseTimes++
			if playerState.GameStatistics.IsPFRChance {
				playerState.GameStatistics.IsPFR = true
			}

			if playerState.GameStatistics.IsATSChance {
				playerState.GameStatistics.IsATS = true
			}

			te.refreshThreeBet(playerState, playerIdx)

			if playerState.GameStatistics.IsCheckRaiseChance {
				playerState.GameStatistics.IsCheckRaise = true
			}
		}

		if playerState.GameStatistics.IsVPIPChance {
			playerState.GameStatistics.IsVPIP = true
		}

		if playerState.GameStatistics.IsCBetChance {
			playerState.GameStatistics.IsCBet = true
		}
	}

	return err
}

func (te *tableEngine) PlayerCheck(playerID string) error {
	te.lock.Lock()
	defer te.lock.Unlock()

	gamePlayerIdx := te.table.FindGamePlayerIdx(playerID)
	if err := te.validateGameMove(gamePlayerIdx); err != nil {
		return err
	}

	playerIdx := te.table.FindPlayerIndexFromGamePlayerIndex(gamePlayerIdx)
	if playerIdx == UnsetValue {
		return ErrGamePlayerNotFound
	}

	gs, err := te.game.Check(gamePlayerIdx)
	if err == nil {
		te.table.State.LastPlayerGameAction = te.createPlayerGameAction(playerID, playerIdx, WagerAction_Check, 0, gs.GetPlayer(gamePlayerIdx))
		te.emitGamePlayerActionEvent(*te.table.State.LastPlayerGameAction)

		playerState := te.table.State.PlayerStates[playerIdx]
		playerState.GameStatistics.ActionTimes++
		playerState.GameStatistics.CheckTimes++
	}

	return err
}

func (te *tableEngine) PlayerFold(playerID string) error {
	te.lock.Lock()
	defer te.lock.Unlock()

	gamePlayerIdx := te.table.FindGamePlayerIdx(playerID)
	if err := te.validateGameMove(gamePlayerIdx); err != nil {
		return err
	}

	playerIdx := te.table.FindPlayerIndexFromGamePlayerIndex(gamePlayerIdx)
	if playerIdx == UnsetValue {
		return ErrGamePlayerNotFound
	}

	gs, err := te.game.Fold(gamePlayerIdx)
	if err == nil {
		te.table.State.LastPlayerGameAction = te.createPlayerGameAction(playerID, playerIdx, WagerAction_Fold, 0, gs.GetPlayer(gamePlayerIdx))
		te.emitGamePlayerActionEvent(*te.table.State.LastPlayerGameAction)

		playerState := te.table.State.PlayerStates[playerIdx]
		playerState.GameStatistics.ActionTimes++
		playerState.GameStatistics.IsFold = true
		playerState.GameStatistics.FoldRound = te.game.GetGameState().Status.Round

		if playerState.GameStatistics.IsFt3BChance {
			playerState.GameStatistics.IsFt3B = true
		}

		if playerState.GameStatistics.IsFt3BChance {
			playerState.GameStatistics.IsFtCB = true
		}
	}

	return err
}

func (te *tableEngine) PlayerPass(playerID string) error {
	te.lock.Lock()
	defer te.lock.Unlock()

	gamePlayerIdx := te.table.FindGamePlayerIdx(playerID)
	if err := te.validateGameMove(gamePlayerIdx); err != nil {
		return err
	}

	playerIdx := te.table.FindPlayerIndexFromGamePlayerIndex(gamePlayerIdx)
	if playerIdx == UnsetValue {
		return ErrGamePlayerNotFound
	}

	gs, err := te.game.Pass(gamePlayerIdx)
	if err == nil {
		te.table.State.LastPlayerGameAction = te.createPlayerGameAction(playerID, playerIdx, "pass", 0, gs.GetPlayer(gamePlayerIdx))
		te.emitGamePlayerActionEvent(*te.table.State.LastPlayerGameAction)
	}

	return err
}
