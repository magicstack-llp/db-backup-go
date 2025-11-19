package data

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Connection represents a database connection configuration
type Connection struct {
	Host            string   `json:"host"`
	Port            int      `json:"port"`
	User            string   `json:"user"`
	Password        string   `json:"password"`
	MysqldumpPath   string   `json:"mysqldump_path,omitempty"`
	ExcludedDBs     []string `json:"excluded_databases,omitempty"`
	StorageDriver   string   `json:"storage_driver,omitempty"`
	Path            string   `json:"path,omitempty"`
	S3Bucket        string   `json:"s3_bucket,omitempty"`
	SSHHost         string   `json:"ssh_host,omitempty"`
	SSHPort         int      `json:"ssh_port,omitempty"`
	SSHUser         string   `json:"ssh_user,omitempty"`
	SSHKeyPath      string   `json:"ssh_key_path,omitempty"`
	BastionHost     string   `json:"bastion_host,omitempty"`
	BastionPort     int      `json:"bastion_port,omitempty"`
	BastionUser     string   `json:"bastion_user,omitempty"`
	BastionKeyPath  string   `json:"bastion_key_path,omitempty"`
}

// ConnectionManager manages database connections stored in JSON format
type ConnectionManager struct {
	connectionsPath string
}

// NewConnectionManager creates a new ConnectionManager instance
func NewConnectionManager(connectionsPath string) (*ConnectionManager, error) {
	if connectionsPath == "" {
		connectionsPath = defaultConnectionsPath()
	}
	
	cm := &ConnectionManager{
		connectionsPath: connectionsPath,
	}
	
	if err := cm.ensureConnectionsFile(); err != nil {
		return nil, err
	}
	
	return cm, nil
}

// defaultConnectionsPath returns the default path for connections.json
func defaultConnectionsPath() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	base := os.Getenv("HOME")
	if xdg != "" {
		base = xdg
	} else {
		base = filepath.Join(base, ".config")
	}
	return filepath.Join(base, "database-backup", "connections.json")
}

// ensureConnectionsFile ensures the connections.json file exists
func (cm *ConnectionManager) ensureConnectionsFile() error {
	cfgDir := filepath.Dir(cm.connectionsPath)
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	
	if _, err := os.Stat(cm.connectionsPath); os.IsNotExist(err) {
		// Create empty connections file
		empty := make(map[string]*Connection)
		return cm.saveConnections(empty)
	}
	
	return nil
}

// loadConnections loads connections from JSON file
func (cm *ConnectionManager) loadConnections() (map[string]*Connection, error) {
	data, err := os.ReadFile(cm.connectionsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]*Connection), nil
		}
		return nil, fmt.Errorf("failed to read connections file: %w", err)
	}
	
	var connections map[string]*Connection
	if err := json.Unmarshal(data, &connections); err != nil {
		return nil, fmt.Errorf("failed to parse connections file: %w", err)
	}
	
	if connections == nil {
		connections = make(map[string]*Connection)
	}
	
	return connections, nil
}

// saveConnections saves connections to JSON file
func (cm *ConnectionManager) saveConnections(connections map[string]*Connection) error {
	data, err := json.MarshalIndent(connections, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal connections: %w", err)
	}
	
	if err := os.WriteFile(cm.connectionsPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write connections file: %w", err)
	}
	
	return nil
}

// AddConnection adds a new connection
func (cm *ConnectionManager) AddConnection(name string, conn *Connection) error {
	connections, err := cm.loadConnections()
	if err != nil {
		return err
	}
	
	if _, exists := connections[name]; exists {
		return fmt.Errorf("connection '%s' already exists", name)
	}
	
	connections[name] = conn
	return cm.saveConnections(connections)
}

// RemoveConnection removes a connection
func (cm *ConnectionManager) RemoveConnection(name string) error {
	connections, err := cm.loadConnections()
	if err != nil {
		return err
	}
	
	if _, exists := connections[name]; !exists {
		return fmt.Errorf("connection '%s' not found", name)
	}
	
	delete(connections, name)
	return cm.saveConnections(connections)
}

// GetConnection gets a connection by name
func (cm *ConnectionManager) GetConnection(name string) (*Connection, error) {
	connections, err := cm.loadConnections()
	if err != nil {
		return nil, err
	}
	
	conn, exists := connections[name]
	if !exists {
		return nil, fmt.Errorf("connection '%s' not found", name)
	}
	
	return conn, nil
}

// ListConnections returns all connection names
func (cm *ConnectionManager) ListConnections() ([]string, error) {
	connections, err := cm.loadConnections()
	if err != nil {
		return nil, err
	}
	
	names := make([]string, 0, len(connections))
	for name := range connections {
		names = append(names, name)
	}
	
	return names, nil
}

// GetAllConnections returns all connections
func (cm *ConnectionManager) GetAllConnections() (map[string]*Connection, error) {
	return cm.loadConnections()
}

// UpdateConnection updates an existing connection
func (cm *ConnectionManager) UpdateConnection(name string, conn *Connection) error {
	connections, err := cm.loadConnections()
	if err != nil {
		return err
	}
	
	if _, exists := connections[name]; !exists {
		return fmt.Errorf("connection '%s' not found", name)
	}
	
	connections[name] = conn
	return cm.saveConnections(connections)
}

