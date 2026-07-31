package sftpproxy

import "testing"

func TestValidateBindAddr_Loopback(t *testing.T) {
	cases := []string{"127.0.0.1", "::1", "localhost"}
	for _, c := range cases {
		if err := ValidateBindAddr(c, false); err != nil {
			t.Errorf("ValidateBindAddr(%q, false) unexpected error: %v", c, err)
		}
	}
}

func TestValidateBindAddr_NonLoopbackRejected(t *testing.T) {
	cases := []string{"0.0.0.0", "192.168.1.1", "10.0.0.5", "::"}
	for _, c := range cases {
		if err := ValidateBindAddr(c, false); err == nil {
			t.Errorf("ValidateBindAddr(%q, false) expected error", c)
		}
	}
}

func TestValidateBindAddr_AllowRemote(t *testing.T) {
	if err := ValidateBindAddr("0.0.0.0", true); err != nil {
		t.Errorf("allowRemote should accept 0.0.0.0: %v", err)
	}
	if err := ValidateBindAddr("192.168.1.1", true); err != nil {
		t.Errorf("allowRemote should accept 192.168.1.1: %v", err)
	}
}

func TestValidateBindAddr_Empty(t *testing.T) {
	if err := ValidateBindAddr("", false); err == nil {
		t.Error("empty bind should be rejected")
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	loop := []string{"127.0.0.1", "::1", "localhost", "127.0.0.1:22"}
	non := []string{"0.0.0.0", "192.168.1.1", "10.0.0.5", "::", "", "  "}
	for _, c := range loop {
		if !IsLoopbackAddr(c) {
			t.Errorf("IsLoopbackAddr(%q) = false, want true", c)
		}
	}
	for _, c := range non {
		if IsLoopbackAddr(c) {
			t.Errorf("IsLoopbackAddr(%q) = true, want false", c)
		}
	}
}
