package pokertable

import (
	"encoding/json"
	"errors"
	"sync"

	"github.com/d-protocol/pokerlib"
	"github.com/d-protocol/syncsaga"
	"github.com/thoas/go-funk"
)

var (
	ErrGamePlayerNotFound      = errors.New("game: player not found")
	ErrGameInvalidAction       = errors.New("game: invalid action")
	ErrGameUnknownEvent        = errors.New("game: unknown event")
	ErrGameUnknownEventHandler = errors.New("game: unknown event handler")
)

type Game interface {
	// Events
	OnAntesReceived(func(*pokerlib.GameState))
	OnBlindsReceived(func(*pokerlib.GameState))
	OnGameStateUpdated(func(*pokerlib.GameState))
	OnGameRoundClosed(func(*pokerlib.GameState))
	OnGameErrorUpdated(func(*pokerlib.GameState, error))

	// Others
	GetGameState() *pokerlib.GameState
	Start() (*pokerlib.GameState, error)
	Next() (*pokerlib.GameState, error)

	// Group Actions
	ReadyForAll() (*pokerlib.GameState, error)
	PayAnte() (*pokerlib.GameState, error)
	PayBlinds() (*pokerlib.GameState, error)

	// Single Actions
	Ready(playerIdx int) (*pokerlib.GameState, error)
	Pay(playerIdx int, chips int64) (*pokerlib.GameState, error)
	Pass(playerIdx int) (*pokerlib.GameState, error)
	Fold(playerIdx int) (*pokerlib.GameState, error)
	Check(playerIdx int) (*pokerlib.GameState, error)
	Call(playerIdx int) (*pokerlib.GameState, error)
	Allin(playerIdx int) (*pokerlib.GameState, error)
	Bet(playerIdx int, chips int64) (*pokerlib.GameState, error)
	Raise(playerIdx int, chipLevel int64) (*pokerlib.GameState, error)
}

type game struct {
	backend            GameBackend
	gs                 *pokerlib.GameState
	opts               *pokerlib.GameOptions
	rg                 *syncsaga.ReadyGroup
	mu                 sync.RWMutex
	isClosed           bool
	incomingStates     chan *pokerlib.GameState
	onAntesReceived    func(*pokerlib.GameState)
	onBlindsReceived   func(*pokerlib.GameState)
	onGameStateUpdated func(*pokerlib.GameState)
	onGameRoundClosed  (func(*pokerlib.GameState))
	onGameErrorUpdated func(*pokerlib.GameState, error)
}

func NewGame(backend GameBackend, opts *pokerlib.GameOptions) *game {
	rg := syncsaga.NewReadyGroup(
		syncsaga.WithTimeout(17, func(rg *syncsaga.ReadyGroup) {
			// Auto Ready By Default
			states := rg.GetParticipantStates()
			for gamePlayerIdx, isReady := range states {
				if !isReady {
					rg.Ready(gamePlayerIdx)
				}
			}
		}),
	)
	return &game{
		backend:            backend,
		opts:               opts,
		rg:                 rg,
		incomingStates:     make(chan *pokerlib.GameState, 1024),
		onAntesReceived:    func(gs *pokerlib.GameState) {},
		onBlindsReceived:   func(gs *pokerlib.GameState) {},
		onGameStateUpdated: func(gs *pokerlib.GameState) {},
		onGameRoundClosed:  func(*pokerlib.GameState) {},
		onGameErrorUpdated: func(gs *pokerlib.GameState, err error) {},
	}
}

func (g *game) OnAntesReceived(fn func(*pokerlib.GameState)) {
	g.onAntesReceived = fn
}

func (g *game) OnBlindsReceived(fn func(*pokerlib.GameState)) {
	g.onBlindsReceived = fn
}

func (g *game) OnGameStateUpdated(fn func(*pokerlib.GameState)) {
	g.onGameStateUpdated = fn
}

func (g *game) OnGameRoundClosed(fn func(*pokerlib.GameState)) {
	g.onGameRoundClosed = fn
}

func (g *game) OnGameErrorUpdated(fn func(*pokerlib.GameState, error)) {
	g.onGameErrorUpdated = fn
}

func (g *game) GetGameState() *pokerlib.GameState {
	return g.gs
}

func (g *game) Start() (*pokerlib.GameState, error) {
	g.runGameStateUpdater()

	gs, err := g.backend.CreateGame(g.opts)
	if err != nil {
		return g.GetGameState(), err
	}

	g.updateGameState(gs)
	return g.GetGameState(), nil
}

func (g *game) Next() (*pokerlib.GameState, error) {
	gs, err := g.backend.Next(g.gs)
	if err != nil {
		return g.GetGameState(), err
	}

	g.updateGameState(gs)
	return g.GetGameState(), nil
}

func (g *game) ReadyForAll() (*pokerlib.GameState, error) {
	gs, err := g.backend.ReadyForAll(g.gs)
	if err != nil {
		return g.GetGameState(), err
	}

	g.updateGameState(gs)
	return g.GetGameState(), nil
}

func (g *game) PayAnte() (*pokerlib.GameState, error) {
	gs, err := g.backend.PayAnte(g.gs)
	if err != nil {
		return g.GetGameState(), err
	}

	g.updateGameState(gs)
	return g.GetGameState(), nil
}

func (g *game) PayBlinds() (*pokerlib.GameState, error) {
	gs, err := g.backend.PayBlinds(g.gs)
	if err != nil {
		return g.GetGameState(), err
	}

	g.updateGameState(gs)
	return g.GetGameState(), nil
}

func (g *game) Ready(playerIdx int) (*pokerlib.GameState, error) {
	if err := g.validateActionMove(playerIdx, Action_Ready); err != nil {
		return g.GetGameState(), err
	}

	g.rg.Ready(int64(playerIdx))
	return g.GetGameState(), nil
}

func (g *game) Pay(playerIdx int, chips int64) (*pokerlib.GameState, error) {
	if err := g.validateActionMove(playerIdx, Action_Pay); err != nil {
		return g.GetGameState(), err
	}

	event, ok := pokerlib.GameEventBySymbol[g.gs.Status.CurrentEvent]
	if !ok {
		return g.GetGameState(), ErrGameUnknownEvent
	}

	// For blinds
	switch event {
	case pokerlib.GameEvent_AnteRequested:
		fallthrough
	case pokerlib.GameEvent_BlindsRequested:
		g.rg.Ready(int64(playerIdx))
		return g.GetGameState(), nil
	}

	gs, err := g.backend.Pay(g.gs, chips)
	if err != nil {
		return g.GetGameState(), err
	}

	g.updateGameState(gs)
	return g.GetGameState(), nil
}

func (g *game) Pass(playerIdx int) (*pokerlib.GameState, error) {
	if err := g.validatePlayMove(playerIdx); err != nil {
		return g.GetGameState(), err
	}

	gs, err := g.backend.Pass(g.gs)
	if err != nil {
		return g.GetGameState(), err
	}

	g.updateGameState(gs)
	return g.GetGameState(), nil
}

func (g *game) Fold(playerIdx int) (*pokerlib.GameState, error) {
	if err := g.validatePlayMove(playerIdx); err != nil {
		return g.GetGameState(), err
	}

	gs, err := g.backend.Fold(g.gs)
	if err != nil {
		return g.GetGameState(), err
	}

	g.updateGameState(gs)
	return g.GetGameState(), nil
}

func (g *game) Check(playerIdx int) (*pokerlib.GameState, error) {
	if err := g.validatePlayMove(playerIdx); err != nil {
		return g.GetGameState(), err
	}

	gs, err := g.backend.Check(g.gs)
	if err != nil {
		return g.GetGameState(), err
	}

	g.updateGameState(gs)
	return g.GetGameState(), nil
}

func (g *game) Call(playerIdx int) (*pokerlib.GameState, error) {
	if err := g.validatePlayMove(playerIdx); err != nil {
		return g.GetGameState(), err
	}

	gs, err := g.backend.Call(g.gs)
	if err != nil {
		return g.GetGameState(), err
	}

	g.updateGameState(gs)
	return g.GetGameState(), nil
}

func (g *game) Allin(playerIdx int) (*pokerlib.GameState, error) {
	if err := g.validatePlayMove(playerIdx); err != nil {
		return g.GetGameState(), err
	}

	gs, err := g.backend.Allin(g.gs)
	if err != nil {
		return g.GetGameState(), err
	}

	g.updateGameState(gs)
	return g.GetGameState(), nil
}

func (g *game) Bet(playerIdx int, chips int64) (*pokerlib.GameState, error) {
	if err := g.validatePlayMove(playerIdx); err != nil {
		return g.GetGameState(), err
	}

	gs, err := g.backend.Bet(g.gs, chips)
	if err != nil {
		return g.GetGameState(), err
	}

	g.updateGameState(gs)
	return g.GetGameState(), nil
}

func (g *game) Raise(playerIdx int, chipLevel int64) (*pokerlib.GameState, error) {
	if err := g.validatePlayMove(playerIdx); err != nil {
		return g.GetGameState(), err
	}

	gs, err := g.backend.Raise(g.gs, chipLevel)
	if err != nil {
		return g.GetGameState(), err
	}

	g.updateGameState(gs)
	return g.GetGameState(), nil
}

func (g *game) validatePlayMove(playerIdx int) error {
	if p := g.gs.GetPlayer(playerIdx); p == nil {
		return ErrGamePlayerNotFound
	}

	if g.gs.Status.CurrentPlayer != playerIdx {
		return ErrGameInvalidAction
	}

	return nil
}

func (g *game) validateActionMove(playerIdx int, action string) error {
	if p := g.gs.GetPlayer(playerIdx); p == nil {
		return ErrGamePlayerNotFound
	}

	if !g.gs.HasAction(playerIdx, action) {
		return ErrGameInvalidAction
	}

	if g.rg == nil {
		return ErrGameInvalidAction
	}

	return nil
}

func (g *game) runGameStateUpdater() {
	go func() {
		for state := range g.incomingStates {
			g.handleGameState(state)
		}
	}()
}

func (g *game) cloneState(gs *pokerlib.GameState) *pokerlib.GameState {
	// clone table state
	data, err := json.Marshal(gs)
	if err != nil {
		return nil
	}

	var state pokerlib.GameState
	json.Unmarshal(data, &state)

	return &state
}

func (g *game) updateGameState(gs *pokerlib.GameState) {
	g.mu.Lock()
	defer g.mu.Unlock()

	state := g.cloneState(gs)
	g.gs = state

	if g.isClosed {
		return
	}

	g.incomingStates <- state
}

func (g *game) handleGameState(gs *pokerlib.GameState) {
	event, ok := pokerlib.GameEventBySymbol[gs.Status.CurrentEvent]
	if !ok {
		g.onGameErrorUpdated(gs, ErrGameUnknownEvent)
		return
	}

	handlers := map[pokerlib.GameEvent]func(*pokerlib.GameState){
		pokerlib.GameEvent_ReadyRequested:  g.onReadyRequested,
		pokerlib.GameEvent_AnteRequested:   g.onAnteRequested,
		pokerlib.GameEvent_BlindsRequested: g.onBlindsRequested,
		pokerlib.GameEvent_RoundClosed:     g.onRoundClosed,
		pokerlib.GameEvent_GameClosed:      g.onGameClosed,
	}
	if handler, exist := handlers[event]; exist {
		handler(gs)
	}
	g.onGameStateUpdated(gs)
}

func (g *game) onReadyRequested(gs *pokerlib.GameState) {
	// Preparing ready group to wait for all player ready
	g.rg.Stop()
	g.rg.OnCompleted(func(rg *syncsaga.ReadyGroup) {
		if _, err := g.ReadyForAll(); err != nil {
			g.onGameErrorUpdated(gs, err)
			return
		}

		// reset AllowedActions
		for _, p := range gs.Players {
			if funk.Contains(p.AllowedActions, Action_Ready) {
				p.AllowedActions = funk.Filter(p.AllowedActions, func(action string) bool {
					return action != Action_Ready
				}).([]string)
			}
		}
	})

	g.rg.ResetParticipants()
	for _, p := range gs.Players {
		g.rg.Add(int64(p.Idx), false)

		// Allow "ready" action
		p.AllowAction(Action_Ready)
	}

	g.rg.Start()
}

func (g *game) onAnteRequested(gs *pokerlib.GameState) {
	if gs.Meta.Ante == 0 {
		return
	}

	// Preparing ready group to wait for ante paid from all player
	g.rg.Stop()
	g.rg.OnCompleted(func(rg *syncsaga.ReadyGroup) {
		gameState, err := g.PayAnte()
		if err != nil {
			g.onGameErrorUpdated(gs, err)
			return
		}

		// emit event
		g.onAntesReceived(gameState)

		// reset AllowedActions
		for _, p := range gs.Players {
			if funk.Contains(p.AllowedActions, Action_Pay) {
				p.AllowedActions = funk.Filter(p.AllowedActions, func(action string) bool {
					return action != Action_Pay
				}).([]string)
			}
		}
	})

	g.rg.ResetParticipants()
	for _, p := range gs.Players {
		g.rg.Add(int64(p.Idx), false)

		// Allow "pay" action
		p.AllowAction(Action_Pay)
	}

	g.rg.Start()
}

func (g *game) onBlindsRequested(gs *pokerlib.GameState) {
	// Preparing ready group to wait for blinds
	g.rg.Stop()
	g.rg.OnCompleted(func(rg *syncsaga.ReadyGroup) {
		gameState, err := g.PayBlinds()
		if err != nil {
			g.onGameErrorUpdated(gs, err)
			return
		}

		// emit event
		g.onBlindsReceived(gameState)

		// reset AllowedActions
		for _, p := range gs.Players {
			if funk.Contains(p.AllowedActions, Action_Pay) {
				p.AllowedActions = funk.Filter(p.AllowedActions, func(action string) bool {
					return action != Action_Pay
				}).([]string)
			}
		}
	})

	g.rg.ResetParticipants()
	for _, p := range gs.Players {
		// Allow "pay" action
		if gs.Meta.Blind.BB > 0 && gs.HasPosition(p.Idx, Position_BB) {
			g.rg.Add(int64(p.Idx), false)
			p.AllowAction(Action_Pay)
		} else if gs.Meta.Blind.SB > 0 && gs.HasPosition(p.Idx, Position_SB) {
			g.rg.Add(int64(p.Idx), false)
			p.AllowAction(Action_Pay)
		} else if gs.Meta.Blind.Dealer > 0 && gs.HasPosition(p.Idx, Position_Dealer) {
			g.rg.Add(int64(p.Idx), false)
			p.AllowAction(Action_Pay)
		}
	}

	g.rg.Start()
}

func (g *game) onRoundClosed(gs *pokerlib.GameState) {
	g.onGameRoundClosed(gs)

	// Next round automatically
	gs, err := g.backend.Next(gs)
	if err != nil {
		g.onGameErrorUpdated(gs, err)
		return
	}

	g.updateGameState(gs)
}

func (g *game) onGameClosed(gs *pokerlib.GameState) {
	if g.isClosed {
		return
	}

	g.isClosed = true
	close(g.incomingStates)
}
