package actor

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	pokertable "github.com/d-protocol/pokertable"
)

const TimeFormat_DDMMYYYYhhmmss = "2006-01-02 15:04:05"

func DebugPrintTableGameOpened(t pokertable.Table) {
	timeString := func(timestamp int64) string {
		return time.Unix(timestamp, 0).Format(TimeFormat_DDMMYYYYhhmmss)
	}

	boolToString := func(value bool) string {
		if value {
			return "O"
		} else {
			return "X"
		}
	}

	fmt.Printf("---------- 第 (%d) 手開局 ----------\n", t.State.GameCount)
	fmt.Println("[Time] ", timeString(time.Now().Unix()))
	fmt.Println("[Table ID] ", t.ID)
	fmt.Println("[Table StartAt] ", timeString(t.State.StartAt))
	fmt.Println("[Table Game Count] ", t.State.GameCount)
	fmt.Println("[Table Players]")
	for _, player := range t.State.PlayerStates {
		seat := "X"
		if player.Seat != -1 {
			seat = strconv.Itoa(player.Seat)
		}
		fmt.Printf("seat: %s [%v], in: %s, participated: %s, player: %s\n", seat, player.Positions, boolToString(player.IsIn), boolToString(player.IsParticipated), player.PlayerID)
	}

	if t.State.CurrentDealerSeat != -1 {
		dealerPlayerIndex := t.State.SeatMap[t.State.CurrentDealerSeat]
		if dealerPlayerIndex == -1 {
			fmt.Println("[Table Current Dealer] X")
		} else {
			fmt.Println("[Table Current Dealer] ", t.State.PlayerStates[dealerPlayerIndex].PlayerID)
		}
	} else {
		fmt.Println("[Table Current Dealer] X")
	}

	if t.State.CurrentSBSeat != -1 {
		sbPlayerIndex := t.State.SeatMap[t.State.CurrentSBSeat]
		if sbPlayerIndex == -1 {
			fmt.Println("[Table Current SB] X")
		} else {
			fmt.Println("[Table Current SB] ", t.State.PlayerStates[sbPlayerIndex].PlayerID)
		}
	} else {
		fmt.Println("[Table Current SB] X")
	}

	if t.State.CurrentBBSeat != -1 {
		bbPlayerIndex := t.State.SeatMap[t.State.CurrentBBSeat]
		if bbPlayerIndex == -1 {
			fmt.Println("[Table Current BB] X")
		} else {
			fmt.Println("[Table Current BB] ", t.State.PlayerStates[bbPlayerIndex].PlayerID)
		}
	} else {
		fmt.Println("[Table Current BB] X")
	}

	fmt.Printf("[Table SeatMap] %+v\n", t.State.SeatMap)
	for Seat, playerIndex := range t.State.SeatMap {
		playerID := "X"
		positions := []string{"Unknown"}
		bankroll := "X"
		if playerIndex != -1 {
			playerID = t.State.PlayerStates[playerIndex].PlayerID
			positions = t.State.PlayerStates[playerIndex].Positions
			bankroll = fmt.Sprintf("%d", t.State.PlayerStates[playerIndex].Bankroll)
		}

		fmt.Printf("seat: %d, position: %v, player: %s, bankroll: %s\n", Seat, positions, playerID, bankroll)
	}

	fmt.Println("[Blind Data]")
	blindLevel := "中場休息"
	if t.State.BlindState.Level != -1 {
		blindLevel = strconv.Itoa(t.State.BlindState.Level)
	}
	fmt.Println("Level: ", blindLevel)
	fmt.Println("Ante: ", t.State.BlindState.Ante)
	fmt.Println("Dealer: ", t.State.BlindState.Dealer)
	fmt.Println("SB: ", t.State.BlindState.SB)
	fmt.Println("BB: ", t.State.BlindState.BB)

	fmt.Println("[Game Players]")
	for _, playerIdx := range t.State.GamePlayerIndexes {
		player := t.State.PlayerStates[playerIdx]
		seat := "X"
		if player.Seat != -1 {
			seat = strconv.Itoa(player.Seat)
		}
		fmt.Printf("seat: %s [%v], player: %s, bankroll: %d\n", seat, player.Positions, player.PlayerID, player.Bankroll)
	}

	fmt.Println()
}

func DebugPrintTableGameSettled(t pokertable.Table) {
	timeString := func(timestamp int64) string {
		return time.Unix(timestamp, 0).Format(TimeFormat_DDMMYYYYhhmmss)
	}

	playerIDMapper := func(t pokertable.Table, gameStatePlayerIdx int) string {
		for gamePlayerIdx, playerIdx := range t.State.GamePlayerIndexes {
			if gamePlayerIdx == gameStatePlayerIdx {
				return t.State.PlayerStates[playerIdx].PlayerID
			}
		}
		return ""
	}

	fmt.Printf("---------- 第 (%d) 手結算 ----------\n", t.State.GameCount)
	fmt.Println("[Time] ", timeString(time.Now().Unix()))
	result := t.State.GameState.Result
	for _, player := range result.Players {
		playerID := playerIDMapper(t, player.Idx)
		fmt.Printf("%s: final: %d, changed: %d\n", playerID, player.Final, player.Changed)
	}
	for idx, pot := range result.Pots {
		fmt.Printf("pot[%d]: %d\n", idx, pot.Total)
		for _, winner := range pot.Winners {
			playerID := playerIDMapper(t, winner.Idx)
			fmt.Printf("%s: withdraw: %d\n", playerID, winner.Withdraw)
		}
		fmt.Println()
	}

	fmt.Println("---------- Player Result ----------")
	for _, player := range t.State.PlayerStates {
		fmt.Printf("%s: seat: %d[%+v], bankroll: %d\n", player.PlayerID, player.Seat, player.Positions, player.Bankroll)
	}

	fmt.Println("---------- Next BB Players ----------")
	if len(t.State.NextBBOrderPlayerIDs) > 0 {
		fmt.Println("Next BB Players:", strings.Join(t.State.NextBBOrderPlayerIDs, ","))
	} else {
		fmt.Println("No Next BB Players")
	}

	fmt.Println()
}
