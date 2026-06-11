// Package protos provides generated Protocol Buffer types and utility functions
// for the MITM communication protocol.
package protos

//go:generate protoc --go_opt=paths=source_relative --experimental_allow_proto3_optional --go_opt=Mrotom.proto=github.com/UnownHash/RotomNG/libs/protos --go_out=. rotom.proto
