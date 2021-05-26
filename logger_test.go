package gozap2seq

import "testing"

func TestBadUrl(t *testing.T) {
	_, err := NewLogInjector("http://///////", "boooo")
	if err == nil {
		t.Error("not handling bad URL properly")
	}
}
