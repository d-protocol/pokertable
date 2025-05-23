package pokertable

import (
	"encoding/json"

	"github.com/d-protocol/pokerlib"
)

type NativeGameBackend struct {
	engine pokerlib.PokerFace
}

func NewNativeGameBackend() *NativeGameBackend {
	return &NativeGameBackend{
		engine: pokerlib.NewPokerFace(),
	}
}

func cloneGameState(gs *pokerlib.GameState) *pokerlib.GameState {
	// Note: we must clone a new structure for preventing original data of game engine is modified outside.
	data, err := json.Marshal(gs)
	if err != nil {
		return nil
	}

	var state pokerlib.GameState
	err = json.Unmarshal([]byte(data), &state)
	if err != nil {
		return nil
	}

	return &state
}

func (ngb *NativeGameBackend) getState(g pokerlib.Game) *pokerlib.GameState {
	return cloneGameState(g.GetState())
}

func (ngb *NativeGameBackend) CreateGame(opts *pokerlib.GameOptions) (*pokerlib.GameState, error) {
	g := ngb.engine.NewGame(opts)
	err := g.Start()
	if err != nil {
		return nil, err
	}
	return ngb.getState(g), nil
}

func (ngb *NativeGameBackend) ReadyForAll(gs *pokerlib.GameState) (*pokerlib.GameState, error) {
	g := ngb.engine.NewGameFromState(cloneGameState(gs))
	err := g.ReadyForAll()
	if err != nil {
		return nil, err
	}
	return ngb.getState(g), nil
}

func (ngb *NativeGameBackend) PayAnte(gs *pokerlib.GameState) (*pokerlib.GameState, error) {
	g := ngb.engine.NewGameFromState(cloneGameState(gs))
	err := g.PayAnte()
	if err != nil {
		return nil, err
	}
	return ngb.getState(g), nil
}

func (ngb *NativeGameBackend) PayBlinds(gs *pokerlib.GameState) (*pokerlib.GameState, error) {
	g := ngb.engine.NewGameFromState(cloneGameState(gs))
	err := g.PayBlinds()
	if err != nil {
		return nil, err
	}
	return ngb.getState(g), nil
}

func (ngb *NativeGameBackend) Next(gs *pokerlib.GameState) (*pokerlib.GameState, error) {
	g := ngb.engine.NewGameFromState(cloneGameState(gs))
	err := g.Next()
	if err != nil {
		return nil, err
	}
	return ngb.getState(g), nil
}

func (ngb *NativeGameBackend) Pay(gs *pokerlib.GameState, chips int64) (*pokerlib.GameState, error) {
	g := ngb.engine.NewGameFromState(cloneGameState(gs))
	err := g.Pay(chips)
	if err != nil {
		return nil, err
	}
	return ngb.getState(g), nil
}

func (ngb *NativeGameBackend) Fold(gs *pokerlib.GameState) (*pokerlib.GameState, error) {
	g := ngb.engine.NewGameFromState(cloneGameState(gs))
	err := g.Fold()
	if err != nil {
		return nil, err
	}
	return ngb.getState(g), nil
}

func (ngb *NativeGameBackend) Check(gs *pokerlib.GameState) (*pokerlib.GameState, error) {
	g := ngb.engine.NewGameFromState(cloneGameState(gs))
	err := g.Check()
	if err != nil {
		return nil, err
	}
	return ngb.getState(g), nil
}

func (ngb *NativeGameBackend) Call(gs *pokerlib.GameState) (*pokerlib.GameState, error) {
	g := ngb.engine.NewGameFromState(cloneGameState(gs))
	err := g.Call()
	if err != nil {
		return nil, err
	}
	return ngb.getState(g), nil
}

func (ngb *NativeGameBackend) Allin(gs *pokerlib.GameState) (*pokerlib.GameState, error) {
	g := ngb.engine.NewGameFromState(cloneGameState(gs))
	err := g.Allin()
	if err != nil {
		return nil, err
	}
	return ngb.getState(g), nil
}

func (ngb *NativeGameBackend) Bet(gs *pokerlib.GameState, chips int64) (*pokerlib.GameState, error) {
	g := ngb.engine.NewGameFromState(cloneGameState(gs))
	err := g.Bet(chips)
	if err != nil {
		return nil, err
	}
	return ngb.getState(g), nil
}

func (ngb *NativeGameBackend) Raise(gs *pokerlib.GameState, chipLevel int64) (*pokerlib.GameState, error) {
	g := ngb.engine.NewGameFromState(cloneGameState(gs))
	err := g.Raise(chipLevel)
	if err != nil {
		return nil, err
	}
	return ngb.getState(g), nil
}

func (ngb *NativeGameBackend) Pass(gs *pokerlib.GameState) (*pokerlib.GameState, error) {
	g := ngb.engine.NewGameFromState(cloneGameState(gs))
	err := g.Pass()
	if err != nil {
		return nil, err
	}
	return ngb.getState(g), nil
}
