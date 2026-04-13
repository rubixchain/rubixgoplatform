package parts

import (
	"github.com/rubixchain/rubixgoplatform/util"
)


type SplitOp struct {
	TokenID            util.TokenID
	ChildrenToTransfer []int // Which child indices (1-based) go to recipient
	ChildrenToKeep     []int // Which child indices stay with sender
}
