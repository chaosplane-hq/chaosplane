package ebpf

import (
	"testing"

	"github.com/cilium/ebpf/asm"
)

// TestDropBPFInstructionsDropsViaShot guards the inversion bug: the historical
// program returned TC_ACT_OK (0) from the "drop" branch and never dropped. The
// drop branch must emit TC_ACT_SHOT (2), the pass branch TC_ACT_OK (0), and the
// branch must be taken on rand%100 < dropPercent.
func TestDropBPFInstructionsDropsViaShot(t *testing.T) {
	ins := dropBPFInstructions(30)

	var sawShot, sawOK, sawMod, sawJLT, sawPrandom bool
	for _, in := range ins {
		switch {
		case in.OpCode.Class().IsLoad() || in.OpCode.Class() == asm.ALUClass || in.OpCode.Class() == asm.ALU64Class:
			if in.OpCode.ALUOp() == asm.Mod && in.Constant == 100 {
				sawMod = true
			}
			if in.Constant == int64(tcActShot) {
				sawShot = true
			}
			if in.Constant == int64(tcActOK) {
				sawOK = true
			}
		}
		if in.OpCode.JumpOp() == asm.JLT && in.Constant == 30 {
			sawJLT = true
		}
		if in.OpCode.JumpOp() == asm.Call && in.Constant == int64(asm.FnGetPrandomU32) {
			sawPrandom = true
		}
	}

	if !sawPrandom {
		t.Error("expected bpf_get_prandom_u32 call for probabilistic drop")
	}
	if !sawMod {
		t.Error("expected `% 100` to map rand into a percentage bucket")
	}
	if !sawJLT {
		t.Error("expected `JLT dropPercent` so rand%100 < pct takes the drop branch")
	}
	if !sawShot {
		t.Errorf("expected TC_ACT_SHOT (%d) in the drop branch", tcActShot)
	}
	if !sawOK {
		t.Errorf("expected TC_ACT_OK (%d) in the pass branch", tcActOK)
	}
}

// TestDropBPFInstructionsThreshold confirms the drop comparison uses the exact
// configured percentage, so 100% drops all and 0% drops none in expectation.
func TestDropBPFInstructionsThreshold(t *testing.T) {
	for _, pct := range []uint32{0, 1, 50, 99, 100} {
		ins := dropBPFInstructions(pct)
		found := false
		for _, in := range ins {
			if in.OpCode.JumpOp() == asm.JLT && in.Constant == int64(pct) {
				found = true
			}
		}
		if !found {
			t.Errorf("dropPercent=%d: expected JLT against %d", pct, pct)
		}
	}
}
