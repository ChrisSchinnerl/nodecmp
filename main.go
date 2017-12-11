package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

// some errors that might occur
var (
	errInvalidArgs = "provided args are invalid: "             // Indicates invalid user input
	errInvalidMD   = "error while reading metadata: "          // Indicates missing or invalid metadata
	errDecode      = "error while decoding entries: "          // Indicates a corrupted json file
	errVersion     = "error while getting version from host: " // Indicates that we couldn't reach host
)

// nodeEntry is a single entry in the node.json file
type nodeEntry struct {
	Address  string `json:"netaddress"`
	Outbound bool   `json:"wasoutboundpeer"`
}

// printUsage prints the usage of the nodecmp tool
func printUsage() {
	fmt.Print("Usage: nodecmp [path1] [path2]")
}

// readPrefix reads an object's prefix
func readPrefix(r io.Reader) (uint64, error) {
	prefix := make([]byte, 8)
	if _, err := r.Read(prefix); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(prefix), nil
}

// writePrefix writes an object's prefix
func writePrefix(w io.Writer, length uint64) error {
	prefix := make([]byte, 8)
	binary.LittleEndian.PutUint64(prefix, length)
	if _, err := w.Write(prefix); err != nil {
		return err
	}
	return nil
}

// nodeVersion gets the version of a node by pinging it
func nodeVersion(addr string) (string, error) {
	// Create dialer
	dialer := &net.Dialer{
		Timeout: 100 * time.Millisecond,
	}

	// Connect to host
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return "", err
	}

	// Send message prefix. 8 bytes version prefix + 5 bytes version
	if err := writePrefix(conn, uint64(13)); err != nil {
		return "", err
	}

	// Send own version prefix
	ownVersion := []byte("1.2.0")
	if err := writePrefix(conn, uint64(len(ownVersion))); err != nil {
		return "", err
	}

	// Send own version
	if _, err := conn.Write(ownVersion); err != nil {
		return "", err
	}

	// Receive peer version prefix
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	prefix, err := readPrefix(conn)
	if err != nil {
		return "", err
	}

	// Receive peer version
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	version := make([]byte, prefix)
	_, err = conn.Read(version)
	if err != nil {
		return "", err
	}
	return string(version), nil
}

func main() {
	// Get commandline args
	args := os.Args[1:]

	// There should be 2 arguments
	if len(args) != 2 {
		printUsage()
		return
	}

	// Read files
	f1, err1 := os.Open(args[0])
	if err1 != nil {
		fmt.Print(errInvalidArgs, err1)
		os.Exit(1)
	}
	f2, err2 := os.Open(args[1])
	if err2 != nil {
		fmt.Print(errInvalidArgs, err2)
		os.Exit(1)
	}

	// Put contents of files in buffers
	r1 := bufio.NewReader(f1)
	r2 := bufio.NewReader(f2)

	// Discard the first three lines since they contain only metadata
	for i := 0; i < 3; i++ {
		_, _, err := r1.ReadLine()
		if err != nil {
			fmt.Print(errInvalidMD, err)
			os.Exit(1)
		}
		_, _, err = r2.ReadLine()
		if err != nil {
			fmt.Print(errInvalidMD, err)
			os.Exit(1)
		}
	}

	// Create decoders
	d1 := json.NewDecoder(r1)
	d2 := json.NewDecoder(r2)

	// Decode entries
	var entries1 []nodeEntry
	var entries2 []nodeEntry
	if err := d1.Decode(&entries1); err != nil {
		fmt.Print(errDecode, err)
		os.Exit(1)
	}
	if err := d2.Decode(&entries2); err != nil {
		fmt.Print(errDecode, err)
		os.Exit(1)
	}

	// Print entries that exist in both files
	entryMap := make(map[string]struct{})
	for _, entry := range entries1 {
		entryMap[entry.Address] = struct{}{}
	}
	for _, entry := range entries2 {
		if _, exists := entryMap[entry.Address]; exists {
			version, err := nodeVersion(entry.Address)
			if err == nil {
				fmt.Printf("%v -> %v\n", entry.Address, version)
			}
		}
	}
}
