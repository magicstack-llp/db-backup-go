package data

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHTunnel manages SSH tunnels for database connections
type SSHTunnel struct {
	sshHost        string
	sshPort        int
	sshUser        string
	sshKeyPath     string
	remoteHost     string
	remotePort     int
	bastionHost    string
	bastionPort    int
	bastionUser    string
	bastionKeyPath string
	
	localPort   int
	server      net.Listener
	bastionConn *ssh.Client
	targetConn  *ssh.Client
	stopChan    chan struct{}
}

// NewSSHTunnel creates a new SSHTunnel instance
func NewSSHTunnel(sshHost string, sshPort int, sshUser string, sshKeyPath string,
	remoteHost string, remotePort int,
	bastionHost string, bastionPort int, bastionUser string, bastionKeyPath string) *SSHTunnel {
	
	if bastionPort == 0 {
		bastionPort = 22
	}
	
	return &SSHTunnel{
		sshHost:        sshHost,
		sshPort:        sshPort,
		sshUser:        sshUser,
		sshKeyPath:     sshKeyPath,
		remoteHost:     remoteHost,
		remotePort:     remotePort,
		bastionHost:    bastionHost,
		bastionPort:    bastionPort,
		bastionUser:    bastionUser,
		bastionKeyPath: bastionKeyPath,
		stopChan:       make(chan struct{}),
	}
}

// findFreePort finds a free local port for the tunnel
func (t *SSHTunnel) findFreePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	
	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

// loadSSHKey loads SSH private key from file
func (t *SSHTunnel) loadSSHKey(keyPath string) (ssh.Signer, error) {
	expandedPath := os.ExpandEnv(keyPath)
	if expandedPath[:2] == "~/" {
		home, _ := os.UserHomeDir()
		expandedPath = filepath.Join(home, expandedPath[2:])
	}
	
	keyData, err := os.ReadFile(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SSH key file: %w", err)
	}
	
	// Try different key types
	key, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SSH key: %w", err)
	}
	
	return key, nil
}

// createSSHClient creates and configures SSH client
func (t *SSHTunnel) createSSHClient(host string, port int, user string, keyPath string) (*ssh.Client, error) {
	key, err := t.loadSSHKey(keyPath)
	if err != nil {
		return nil, err
	}
	
	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(key)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}
	
	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s:%d: %w", host, port, err)
	}
	
	return client, nil
}

// Start starts the SSH tunnel and returns local port
func (t *SSHTunnel) Start() (int, error) {
	if t.localPort != 0 {
		return t.localPort, nil
	}
	
	port, err := t.findFreePort()
	if err != nil {
		return 0, fmt.Errorf("failed to find free port: %w", err)
	}
	t.localPort = port
	
	if t.bastionHost != "" {
		// Double hop: connect through bastion to target
		// Step 1: Connect to bastion
		bastionUser := t.bastionUser
		if bastionUser == "" {
			bastionUser = t.sshUser
		}
		bastionKeyPath := t.bastionKeyPath
		if bastionKeyPath == "" {
			bastionKeyPath = t.sshKeyPath
		}
		
		bastionClient, err := t.createSSHClient(t.bastionHost, t.bastionPort, bastionUser, bastionKeyPath)
		if err != nil {
			return 0, fmt.Errorf("failed to connect to bastion: %w", err)
		}
		t.bastionConn = bastionClient
		
		// Step 2: Create channel through bastion to target SSH server
		conn, err := bastionClient.Dial("tcp", fmt.Sprintf("%s:%d", t.sshHost, t.sshPort))
		if err != nil {
			bastionClient.Close()
			return 0, fmt.Errorf("failed to dial target through bastion: %w", err)
		}
		
		// Step 3: Create SSH transport over the channel
		key, err := t.loadSSHKey(t.sshKeyPath)
		if err != nil {
			conn.Close()
			bastionClient.Close()
			return 0, fmt.Errorf("failed to load SSH key: %w", err)
		}
		config := &ssh.ClientConfig{
			User:            t.sshUser,
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(key)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}
		
		ncc, chans, reqs, err := ssh.NewClientConn(conn, fmt.Sprintf("%s:%d", t.sshHost, t.sshPort), config)
		if err != nil {
			conn.Close()
			bastionClient.Close()
			return 0, fmt.Errorf("failed to create SSH connection through bastion: %w", err)
		}
		
		targetClient := ssh.NewClient(ncc, chans, reqs)
		t.targetConn = targetClient
	} else {
		// Simple SSH tunnel: direct connection
		targetClient, err := t.createSSHClient(t.sshHost, t.sshPort, t.sshUser, t.sshKeyPath)
		if err != nil {
			return 0, fmt.Errorf("failed to connect to SSH host: %w", err)
		}
		t.targetConn = targetClient
	}
	
	// Create local listener
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", t.localPort))
	if err != nil {
		t.Stop()
		return 0, fmt.Errorf("failed to create local listener: %w", err)
	}
	t.server = listener
	
	// Start forwarding goroutine
	go t.forwardTunnel()
	
	// Wait a moment to ensure tunnel is ready
	time.Sleep(500 * time.Millisecond)
	
	return t.localPort, nil
}

// forwardTunnel forwards local port to remote host through SSH
func (t *SSHTunnel) forwardTunnel() {
	for {
		select {
		case <-t.stopChan:
			return
		default:
		}
		
		// Set deadline for accept
		t.server.(*net.TCPListener).SetDeadline(time.Now().Add(1 * time.Second))
		clientConn, err := t.server.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if t.stopChan != nil {
				select {
				case <-t.stopChan:
					return
				default:
				}
			}
			continue
		}
		
		// Create channel through SSH
		// Format: uint32 host length, host string, uint32 port, uint32 originator IP length, originator IP, uint32 originator port
		hostBytes := []byte(t.remoteHost)
		channelData := make([]byte, 4+len(hostBytes)+4+4+4+4)
		binary.BigEndian.PutUint32(channelData[0:4], uint32(len(hostBytes)))
		copy(channelData[4:], hostBytes)
		binary.BigEndian.PutUint32(channelData[4+len(hostBytes):4+len(hostBytes)+4], uint32(t.remotePort))
		// Originator address (127.0.0.1)
		originatorIP := []byte{127, 0, 0, 1}
		binary.BigEndian.PutUint32(channelData[4+len(hostBytes)+4:4+len(hostBytes)+8], uint32(len(originatorIP)))
		copy(channelData[4+len(hostBytes)+8:], originatorIP)
		binary.BigEndian.PutUint32(channelData[4+len(hostBytes)+8+len(originatorIP):], 0) // originator port
		
		channel, reqs, err := t.targetConn.OpenChannel("direct-tcpip", channelData)
		if err != nil {
			clientConn.Close()
			continue
		}
		
		go ssh.DiscardRequests(reqs)
		
		// Forward data between connections
		go func() {
			defer channel.Close()
			defer clientConn.Close()
			_, _ = io.Copy(channel, clientConn)
		}()
		go func() {
			defer channel.Close()
			defer clientConn.Close()
			_, _ = io.Copy(clientConn, channel)
		}()
	}
}

// Stop stops the SSH tunnel
func (t *SSHTunnel) Stop() {
	close(t.stopChan)
	
	if t.server != nil {
		t.server.Close()
		t.server = nil
	}
	
	if t.targetConn != nil {
		t.targetConn.Close()
		t.targetConn = nil
	}
	
	if t.bastionConn != nil {
		t.bastionConn.Close()
		t.bastionConn = nil
	}
	
	t.localPort = 0
}

