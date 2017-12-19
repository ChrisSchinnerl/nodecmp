package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
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
	fmt.Print("Usage: nodecmp [path1] [path2] ... [pathN]")
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
		Timeout: time.Minute,
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
	prefix, err := readPrefix(conn)
	if err != nil {
		return "", err
	}

	// Receive peer version
	version := make([]byte, prefix)
	_, err = conn.Read(version)
	if err != nil {
		return "", err
	}
	return string(version), nil
}

// loadNodes reads a nodes file and returns the entries
func loadNodes(path string) map[string]bool {
	// Read file
	f, err := os.Open(path)
	if err != nil {
		fmt.Print(errInvalidArgs, err)
		os.Exit(1)
	}

	// Put contents of file in buffer
	r := bufio.NewReader(f)

	// Discard the first three lines since they contain only metadata
	for i := 0; i < 3; i++ {
		_, _, err := r.ReadLine()
		if err != nil {
			fmt.Print(errInvalidMD, err)
			os.Exit(1)
		}
	}

	// Create decoder
	d := json.NewDecoder(r)

	// Decode entries
	var entries []nodeEntry
	if err := d.Decode(&entries); err != nil {
		fmt.Print(errDecode, err)
		os.Exit(1)
	}

	// Create a set out of the entries
	entrySet := make(map[string]bool)
	for _, entry := range entries {
		entrySet[entry.Address] = entry.Outbound
	}
	return entrySet
}

// intersect intersects 2 maps
func intersect(m1 map[string]bool, m2 map[string]bool) map[string]bool {
	intersected := make(map[string]bool)
	for key, value := range m1 {
		if _, exists := m2[key]; exists {
			intersected[key] = value
		}
	}
	return intersected
}

func main() {
	// Get commandline args
	args := os.Args[1:]

	// There should be 2 or more
	if len(args) < 2 {
		printUsage()
		return
	}

	// Pairwise intersect all entries
	entryMap := loadNodes(args[0])
	for _, path := range args[1:] {
		entryMap = intersect(entryMap, loadNodes(path))
	}

	var wg sync.WaitGroup
	for address := range entryMap {
		wg.Add(1)
		go func(a string) {
			version, err := nodeVersion(a)
			if err == nil {
				fmt.Printf("%v -> %v\n", a, version)
			}
			wg.Done()
		}(address)
	}
	wg.Wait()
}
