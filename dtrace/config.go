package dtrace

// Config settings
type Config struct {
	Enable            bool     `yaml:"enable"`
	Address           string   `yaml:"address"`
	MaxActiveRequests int64    `yaml:"max_active_requests"`
	ElasticSearch     []string `yaml:"elasticsearch"`
	BulkSize          int      `yaml:"bulk_size"`
	Sampling          uint64   `yaml:"sampling"`
	Replicas          []string `yaml:"replicas"`
	MyReplicaIndex    int      `yaml:"my_replica_index"`
	MinTTL            int      `yaml:"min_ttl"`
	MaxTTL            int      `yaml:"max_ttl"`
}
