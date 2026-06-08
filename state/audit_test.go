package state

import "testing"

func TestChainHashDeterministicAndSensitive(t *testing.T) {
	base := ChainHash("t1", "intent.applied", "reroute", "")
	if base != ChainHash("t1", "intent.applied", "reroute", "") {
		t.Fatal("ChainHash must be deterministic")
	}
	if ChainHash("t1", "intent.applied", "EVIL", "") == base {
		t.Fatal("changing detail must change the hash")
	}
	if ChainHash("t1", "intent.applied", "reroute", "x") == base {
		t.Fatal("changing prev must change the hash")
	}
	if ChainHash("t2", "intent.applied", "reroute", "") == base {
		t.Fatal("changing ts must change the hash")
	}
}
