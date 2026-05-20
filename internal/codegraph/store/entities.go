package store

import "time"

//go:generate go run github.com/objectbox/objectbox-go/cmd/objectbox-gogen

type MetadataEntity struct {
	Id            uint64
	SchemaVersion int
	SourcePath    string `objectbox:"index"`
	Status        string `objectbox:"index"`
	ErrorText     string
	StartedAt     time.Time `objectbox:"date"`
	CompletedAt   time.Time `objectbox:"date"`
}

type FileEntity struct {
	Id          uint64
	Path        string `objectbox:"index"`
	Language    string `objectbox:"index"`
	ContentHash string
	Size        int64
	ModifiedAt  time.Time `objectbox:"date"`
	IndexedAt   time.Time `objectbox:"date"`
	NodeCount   int
}

type NodeEntity struct {
	Id            uint64
	StableID      string `objectbox:"unique"`
	Kind          string `objectbox:"index"`
	Name          string `objectbox:"index"`
	QualifiedName string `objectbox:"index"`
	FilePath      string `objectbox:"index"`
	Language      string `objectbox:"index"`
	StartLine     int
	EndLine       int
	StartColumn   int
	EndColumn     int
	Signature     string
}

type EdgeEntity struct {
	Id             uint64
	SourceStableID string `objectbox:"index"`
	TargetStableID string `objectbox:"index"`
	Kind           string `objectbox:"index"`
	FilePath       string `objectbox:"index"`
	Line           int
	Column         int
	Provenance     string
}

type UnresolvedReferenceEntity struct {
	Id            uint64
	FromStableID  string `objectbox:"index"`
	ReferenceName string `objectbox:"index"`
	ReferenceKind string `objectbox:"index"`
	FilePath      string `objectbox:"index"`
	Language      string `objectbox:"index"`
	Line          int
	Column        int
}
