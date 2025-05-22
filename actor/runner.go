package actor

import (
	"github.com/d-protocol/pokertable"
)

type Runner interface {
	SetActor(a Actor)
	UpdateTableState(t *pokertable.Table) error
}
