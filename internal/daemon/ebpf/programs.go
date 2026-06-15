package ebpf

import (
	"github.com/cilium/ebpf/asm"
)

func delayBPFInstructions(delayUS uint32) asm.Instructions {
	return asm.Instructions{
		asm.Mov.Imm(asm.R1, int32(delayUS*1000)),
		asm.FnKtimeGetNs.Call(),
		asm.Mov.Reg(asm.R6, asm.R0),

		asm.FnKtimeGetNs.Call(),
		asm.Sub.Reg(asm.R0, asm.R6),
		asm.JGT.Imm(asm.R0, int32(delayUS*1000), "pass"),

		asm.Mov.Imm(asm.R0, 0).WithSymbol("drop"),
		asm.Return(),

		asm.Mov.Imm(asm.R0, -1).WithSymbol("pass"),
		asm.Return(),
	}
}

func dropBPFInstructions(dropPercent uint32) asm.Instructions {
	return asm.Instructions{
		asm.FnGetPrandomU32.Call(),

		asm.Mod.Imm(asm.R0, 100),

		asm.JGT.Imm(asm.R0, int32(dropPercent), "pass"),

		asm.Mov.Imm(asm.R0, 0).WithSymbol("drop"),
		asm.Return(),

		asm.Mov.Imm(asm.R0, -1).WithSymbol("pass"),
		asm.Return(),
	}
}
