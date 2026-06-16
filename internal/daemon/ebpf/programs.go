package ebpf

import (
	"github.com/cilium/ebpf/asm"
)

// TC action return codes from uapi/linux/pkt_cls.h, returned by a SchedCLS
// classifier to tell the datapath what to do with the packet.
const (
	tcActOK   = 0 // let the packet continue
	tcActShot = 2 // drop the packet
)

// dropBPFInstructions builds a tc classifier that drops dropPercent% of packets.
//
// A TC classifier cannot sleep or queue, so probabilistic drop is the only loss
// primitive expressible here. bpf_get_prandom_u32 % 100 is uniform in [0,99], so
// dropping when that value < dropPercent drops dropPercent% in expectation.
func dropBPFInstructions(dropPercent uint32) asm.Instructions {
	return asm.Instructions{
		asm.FnGetPrandomU32.Call(),
		asm.Mod.Imm(asm.R0, 100),
		asm.JLT.Imm(asm.R0, int32(dropPercent), "drop"),

		asm.Mov.Imm(asm.R0, tcActOK),
		asm.Return(),

		asm.Mov.Imm(asm.R0, tcActShot).WithSymbol("drop"),
		asm.Return(),
	}
}
