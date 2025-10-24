package mods

import "errors"

// ErrStageAlreadyClaimed signals the stage was claimed by another worker.
var ErrStageAlreadyClaimed = errors.New("mods: stage already claimed")

// ErrTicketNotFound signals the requested ticket could not be located.
var ErrTicketNotFound = errors.New("mods: ticket not found")

// ErrStageNotFound signals the requested stage was not present in the ticket graph.
var ErrStageNotFound = errors.New("mods: stage not found")
