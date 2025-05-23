package pokertable

import (
	"errors"
	"fmt"
	"time"

	"github.com/d-protocol/pokerlib"
	"github.com/d-protocol/pokerlib/settlement"
	"github.com/thoas/go-funk"
)

func (te *tableEngine) tableGameOpen() error {
	te.lock.Lock()
	defer te.lock.Unlock()

	if te.table.State.GameState != nil {
		fmt.Printf("[DEBUG#tableGameOpen] Table (%s) game (%s) with game count (%d) is already opened.\n", te.table.ID, te.table.State.GameState.GameID, te.table.State.GameCount)
		return nil
	}

	// Start the game
	newTable, err := te.openGame(te.table)

	retry := 10
	if err != nil {
		// Retry opening the game within 30 seconds
		if errors.Is(err, ErrTableOpenGameFailed) {
			reopened := false

			for i := 0; i < retry; i++ {
				time.Sleep(time.Second * 3)

				// Game already started, do nothing
				gameStartingStatuses := []TableStateStatus{
					TableStateStatus_TableGameOpened,
					TableStateStatus_TableGamePlaying,
					TableStateStatus_TableGameSettled,
				}
				isGameRunning := funk.Contains(gameStartingStatuses, te.table.State.Status)
				if isGameRunning {
					return nil
				}

				newTable, err = te.openGame(te.table)
				if err != nil {
					if errors.Is(err, ErrTableOpenGameFailed) {
						fmt.Printf("table (%s): failed to open game. retry %d time(s)...\n", te.table.ID, i+1)
						continue
					} else if errors.Is(err, ErrTableOpenGameFailedInBlindBreakingLevel) {
						// Already in a break, do nothing
						fmt.Printf("table (%s): failed to open game when blind level is negative\n", te.table.ID)
						return nil
					} else {
						return err
					}
				} else {
					reopened = true
					break
				}
			}

			if !reopened {
				return err
			}
		} else if errors.Is(err, ErrTableOpenGameFailedInBlindBreakingLevel) {
			// Already in a break, do nothing
			fmt.Printf("table (%s): failed to open game when blind level is negative\n", te.table.ID)
			return nil
		} else {
			return err
		}
	}
	te.table = newTable
	te.emitEvent("tableGameOpen", "")

	// Start the game engine for this hand
	return te.startGame()
}

func (te *tableEngine) openGame(oldTable *Table) (*Table, error) {
	// Step 1: Check TableState
	if !oldTable.State.BlindState.IsSet() {
		return oldTable, ErrTableOpenGameFailed
	}

	if oldTable.State.BlindState.IsBreaking() {
		return oldTable, ErrTableOpenGameFailedInBlindBreakingLevel
	}

	// Step 2: Clone Table for calculation
	cloneTable, err := oldTable.Clone()
	if err != nil {
		return oldTable, err
	}

	// Step 3: Update status
	cloneTable.State.Status = TableStateStatus_TableGameOpened

	// Step 4: Calculate seats
	if !te.sm.IsInitPositions() {
		if err := te.sm.InitPositions(true); err != nil {
			return oldTable, ErrTableOpenGameFailed
		}
	} else {
		if err := te.sm.RotatePositions(); err != nil {
			return oldTable, ErrTableOpenGameFailed
		}
	}

	// Step 5: Update information about players participating in this hand
	// update player is_participated
	for i := 0; i < len(cloneTable.State.PlayerStates); i++ {
		player := cloneTable.State.PlayerStates[i]
		active, err := te.sm.IsPlayerActive(player.PlayerID)
		if err != nil {
			return oldTable, err
		}
		player.IsParticipated = active
	}

	// update gamePlayerIndexes & positions
	cloneTable.State.GamePlayerIndexes = te.calcGamePlayerIndexes(
		cloneTable.Meta.Rule,
		cloneTable.Meta.TableMaxSeatCount,
		te.sm.CurrentDealerSeatID(),
		te.sm.CurrentSBSeatID(),
		te.sm.CurrentBBSeatID(),
		cloneTable.State.SeatMap,
		cloneTable.State.PlayerStates,
	)

	// update player positions
	te.updatePlayerPositions(cloneTable.Meta.TableMaxSeatCount, cloneTable.State.PlayerStates)

	// Step 6: Update table state (GameCount & current Dealer & BB positions)
	cloneTable.State.GameCount = cloneTable.State.GameCount + 1
	cloneTable.State.CurrentDealerSeat = te.sm.CurrentDealerSeatID()
	cloneTable.State.CurrentSBSeat = te.sm.CurrentSBSeatID()
	cloneTable.State.CurrentBBSeat = te.sm.CurrentBBSeatID()

	return cloneTable, nil
}

func (te *tableEngine) startGame() error {
	rule := te.table.Meta.Rule
	blind := te.table.State.BlindState

	// create game options
	opts := pokerlib.NewStardardGameOptions()
	opts.Deck = pokerlib.NewStandardDeckCards()

	if rule == CompetitionRule_ShortDeck {
		opts = pokerlib.NewShortDeckGameOptions()
		opts.Deck = pokerlib.NewShortDeckCards()
	} else if rule == CompetitionRule_Omaha {
		opts.HoleCardsCount = 4
		opts.RequiredHoleCardsCount = 2
	}

	// preparing blind
	opts.Ante = blind.Ante
	opts.Blind = pokerlib.BlindSetting{
		Dealer: blind.Dealer,
		SB:     blind.SB,
		BB:     blind.BB,
	}

	// preparing players
	playerSettings := make([]*pokerlib.PlayerSetting, 0)
	for _, playerIdx := range te.table.State.GamePlayerIndexes {
		player := te.table.State.PlayerStates[playerIdx]
		playerSettings = append(playerSettings, &pokerlib.PlayerSetting{
			Bankroll:  player.Bankroll,
			Positions: player.Positions,
		})
	}
	if !funk.Contains(playerSettings[0].Positions, Position_Dealer) {
		playerSettings[0].Positions = append(playerSettings[0].Positions, Position_Dealer)
	}
	opts.Players = playerSettings

	// create game
	te.game = NewGame(te.gameBackend, opts)
	te.game.OnGameStateUpdated(func(gs *pokerlib.GameState) {
		te.updateGameState(gs)
	})
	te.game.OnGameErrorUpdated(func(gs *pokerlib.GameState, err error) {
		te.table.State.GameState = gs
		go te.emitErrorEvent("OnGameErrorUpdated", "", err)
	})
	te.game.OnAntesReceived(func(gs *pokerlib.GameState) {
		for gpIdx, p := range gs.Players {
			if playerIdx := te.table.FindPlayerIndexFromGamePlayerIndex(gpIdx); playerIdx != UnsetValue {
				player := te.table.State.PlayerStates[playerIdx]
				pga := te.createPlayerGameAction(player.PlayerID, playerIdx, "pay", player.Bankroll, p)
				pga.Round = "ante"
				te.emitGamePlayerActionEvent(*pga)
			}
		}
	})
	te.game.OnBlindsReceived(func(gs *pokerlib.GameState) {
		for gpIdx, p := range gs.Players {
			for _, pos := range p.Positions {
				if funk.Contains([]string{Position_SB, Position_BB}, pos) {
					if playerIdx := te.table.FindPlayerIndexFromGamePlayerIndex(gpIdx); playerIdx != UnsetValue {
						player := te.table.State.PlayerStates[playerIdx]
						pga := te.createPlayerGameAction(player.PlayerID, playerIdx, "pay", player.Bankroll, p)
						te.emitGamePlayerActionEvent(*pga)
					}
				}
			}
		}
	})
	te.game.OnGameRoundClosed(func(gs *pokerlib.GameState) {
		te.table.State.CurrentActionEndAt = 0
	})

	// start game
	if _, err := te.game.Start(); err != nil {
		return err
	}

	te.table.State.Status = TableStateStatus_TableGamePlaying
	te.table.State.GameBlindState = &TableBlindState{
		Level:  blind.Level,
		Ante:   blind.Ante,
		Dealer: blind.Dealer,
		SB:     blind.SB,
		BB:     blind.BB,
	}
	return nil
}

func (te *tableEngine) settleGame() []*TablePlayerState {
	te.table.State.Status = TableStateStatus_TableGameSettled

	// Calculate showdown winning chance
	notFoldCount := 0
	for _, result := range te.table.State.GameState.Result.Players {
		p := te.table.State.GameState.GetPlayer(result.Idx)
		if p != nil && !p.Fold {
			notFoldCount++
		}
	}

	// Calculate winners
	rank := settlement.NewRank()
	for _, player := range te.table.State.GameState.Players {
		if !player.Fold {
			rank.AddContributor(player.Combination.Power, player.Idx)
		}
	}
	rank.Calculate()
	winnerGamePlayerIndexes := rank.GetWinners()
	winnerPlayerIndexes := make(map[int]bool)
	for _, winnerGamePlayerIndex := range winnerGamePlayerIndexes {
		playerIdx := te.table.FindPlayerIndexFromGamePlayerIndex(winnerGamePlayerIndex)
		if playerIdx == UnsetValue {
			fmt.Printf("[DEBUGsettleGame] can't find player index from game player index (%d)", winnerGamePlayerIndex)
			continue
		}

		winnerPlayerIndexes[playerIdx] = true
	}

	// Update player chips based on win/loss to their bankroll
	alivePlayers := make([]*TablePlayerState, 0)
	for _, player := range te.table.State.GameState.Result.Players {
		playerIdx := te.table.State.GamePlayerIndexes[player.Idx]
		playerState := te.table.State.PlayerStates[playerIdx]
		playerState.Bankroll = player.Final

		// Update player showdown winning chance
		p := te.table.State.GameState.GetPlayer(player.Idx)
		if p != nil && !p.Fold && notFoldCount > 1 {
			playerState.GameStatistics.ShowdownWinningChance = true
			if _, exist := winnerPlayerIndexes[playerIdx]; exist {
				playerState.GameStatistics.IsShowdownWinning = true
			}
		} else {
			playerState.GameStatistics.ShowdownWinningChance = false
		}

		if playerState.Bankroll > 0 {
			alivePlayers = append(alivePlayers, playerState)
		}
	}

	// Update NextBBOrderPlayerIDs (remove players without chips)
	te.table.State.NextBBOrderPlayerIDs = te.refreshNextBBOrderPlayerIDs(te.sm.CurrentBBSeatID(), te.table.Meta.TableMaxSeatCount, te.table.State.PlayerStates, te.table.State.SeatMap)

	te.emitEvent("SettleTableGameResult", "")
	te.emitTableStateEvent(TableStateEvent_GameSettled)

	return alivePlayers
}

func (te *tableEngine) continueGame(alivePlayers []*TablePlayerState) error {
	// Reset table state
	te.table.State.Status = TableStateStatus_TableGameStandby
	te.table.State.GamePlayerIndexes = make([]int, 0)
	te.table.State.NextBBOrderPlayerIDs = make([]string, 0)
	te.table.State.CurrentActionEndAt = 0
	te.table.State.GameState = nil
	te.table.State.LastPlayerGameAction = nil
	for i := 0; i < len(te.table.State.PlayerStates); i++ {
		playerState := te.table.State.PlayerStates[i]
		playerState.Positions = make([]string, 0)
		playerState.GameStatistics = NewPlayerGameStatistics()
		if err := te.sm.UpdatePlayerHasChips(playerState.PlayerID, playerState.Bankroll > 0); err != nil {
			return err
		}
		active, err := te.sm.IsPlayerActive(playerState.PlayerID)
		if err != nil {
			return err
		}

		playerState.IsParticipated = active
	}

	var nextMoveInterval int
	var nextMoveHandler func() error

	// Table time is up, do not automatically open the next hand (CT/Cash)
	ctMTTAutoGameOpenEnd := false
	if te.table.Meta.Mode == CompetitionMode_CT || te.table.Meta.Mode == CompetitionMode_Cash {
		tableEndAt := time.Unix(te.table.State.StartAt, 0).Add(time.Second * time.Duration(te.table.Meta.MaxDuration)).Unix()
		ctMTTAutoGameOpenEnd = time.Now().Unix() > tableEndAt
	}

	if ctMTTAutoGameOpenEnd {
		nextMoveInterval = 1
		nextMoveHandler = func() error {
			fmt.Printf("[DEBUG#continueGame] delay -> not auto opened %s table (%s), end: %s, now: %s\n", te.table.Meta.Mode, te.table.ID, time.Unix(te.table.State.StartAt, 0).Add(time.Second*time.Duration(te.table.Meta.MaxDuration)), time.Now())
			te.onAutoGameOpenEnd(te.table.Meta.CompetitionID, te.table.ID)
			return nil
		}
	} else {
		nextMoveInterval = te.options.GameContinueInterval
		nextMoveHandler = func() error {
			// If the table is closed during the Interval, do not continue
			if te.table.State.Status == TableStateStatus_TableClosed {
				return nil
			}

			// If the table is released during the Interval, do not continue
			if te.isReleased {
				return nil
			}

			// Table continuation: pause or open
			if te.table.ShouldPause() {
				// Pause processing
				te.table.State.Status = TableStateStatus_TablePausing
				te.emitEvent("ContinueGame -> Pause", "")
				te.emitTableStateEvent(TableStateEvent_StatusUpdated)
			} else {
				if te.shouldAutoGameOpen() {
					// Setup next game
					nextGameCount := te.table.State.GameCount + 1
					participants := make(map[string]int)
					for idx, player := range alivePlayers {
						participants[player.PlayerID] = idx
					}
					te.SetUpTableGame(nextGameCount, participants)
					return nil
				}

				// Unhandled Situation
				str, _ := te.table.GetJSON()
				fmt.Printf("[DEBUG#continueGame] delay -> unhandled issue. Table: %s\n", str)
			}
			return nil
		}
	}

	return te.delay(nextMoveInterval, nextMoveHandler)
}
