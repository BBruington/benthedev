package app

import (
	"github.com/google/uuid"
)

// bodyOutput and bodyInput wrap a core DTO as a huma response or request body. huma names the schema
// after the DTO, so the wrappers never appear in the generated TypeScript.
type bodyOutput[Body any] struct {
	Body Body
}

type bodyInput[Body any] struct {
	Body Body
}

type memberPath struct {
	MemberID uuid.UUID `path:"memberId" format:"uuid"`
}

type memberBodyInput[Body any] struct {
	MemberID uuid.UUID `path:"memberId" format:"uuid"`
	Body     Body
}
