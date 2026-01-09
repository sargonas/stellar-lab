package main

import (
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ProtocolVersion
		wantErr bool
	}{
		{
			name:  "valid version",
			input: "1.2.3",
			want:  ProtocolVersion{Major: 1, Minor: 2, Patch: 3},
		},
		{
			name:  "version with v prefix",
			input: "v1.6.0",
			want:  ProtocolVersion{Major: 1, Minor: 6, Patch: 0},
		},
		{
			name:  "zero version",
			input: "0.0.0",
			want:  ProtocolVersion{Major: 0, Minor: 0, Patch: 0},
		},
		{
			name:  "large numbers",
			input: "10.20.30",
			want:  ProtocolVersion{Major: 10, Minor: 20, Patch: 30},
		},
		{
			name:    "invalid - too few parts",
			input:   "1.2",
			wantErr: true,
		},
		{
			name:    "invalid - too many parts",
			input:   "1.2.3.4",
			wantErr: true,
		},
		{
			name:    "invalid - non-numeric major",
			input:   "a.2.3",
			wantErr: true,
		},
		{
			name:    "invalid - non-numeric minor",
			input:   "1.b.3",
			wantErr: true,
		},
		{
			name:    "invalid - non-numeric patch",
			input:   "1.2.c",
			wantErr: true,
		},
		{
			name:    "invalid - empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid - dev build",
			input:   "dev",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseVersion(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestProtocolVersion_String(t *testing.T) {
	tests := []struct {
		version ProtocolVersion
		want    string
	}{
		{ProtocolVersion{1, 2, 3}, "1.2.3"},
		{ProtocolVersion{0, 0, 0}, "0.0.0"},
		{ProtocolVersion{10, 20, 30}, "10.20.30"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.version.String(); got != tt.want {
				t.Errorf("ProtocolVersion.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProtocolVersion_IsCompatibleWith(t *testing.T) {
	tests := []struct {
		name  string
		v1    ProtocolVersion
		v2    ProtocolVersion
		want  bool
	}{
		{
			name: "same version",
			v1:   ProtocolVersion{1, 6, 0},
			v2:   ProtocolVersion{1, 6, 0},
			want: true,
		},
		{
			name: "same major different minor",
			v1:   ProtocolVersion{1, 6, 0},
			v2:   ProtocolVersion{1, 5, 0},
			want: true,
		},
		{
			name: "same major different patch",
			v1:   ProtocolVersion{1, 6, 0},
			v2:   ProtocolVersion{1, 6, 5},
			want: true,
		},
		{
			name: "different major - incompatible",
			v1:   ProtocolVersion{1, 6, 0},
			v2:   ProtocolVersion{2, 0, 0},
			want: false,
		},
		{
			name: "v0 vs v1 - incompatible",
			v1:   ProtocolVersion{0, 9, 0},
			v2:   ProtocolVersion{1, 0, 0},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v1.IsCompatibleWith(tt.v2); got != tt.want {
				t.Errorf("%v.IsCompatibleWith(%v) = %v, want %v", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestProtocolVersion_IsNewerThan(t *testing.T) {
	tests := []struct {
		name string
		v1   ProtocolVersion
		v2   ProtocolVersion
		want bool
	}{
		{
			name: "same version - not newer",
			v1:   ProtocolVersion{1, 6, 0},
			v2:   ProtocolVersion{1, 6, 0},
			want: false,
		},
		{
			name: "higher major",
			v1:   ProtocolVersion{2, 0, 0},
			v2:   ProtocolVersion{1, 9, 9},
			want: true,
		},
		{
			name: "lower major",
			v1:   ProtocolVersion{1, 9, 9},
			v2:   ProtocolVersion{2, 0, 0},
			want: false,
		},
		{
			name: "same major higher minor",
			v1:   ProtocolVersion{1, 7, 0},
			v2:   ProtocolVersion{1, 6, 0},
			want: true,
		},
		{
			name: "same major lower minor",
			v1:   ProtocolVersion{1, 5, 0},
			v2:   ProtocolVersion{1, 6, 0},
			want: false,
		},
		{
			name: "same major/minor higher patch",
			v1:   ProtocolVersion{1, 6, 1},
			v2:   ProtocolVersion{1, 6, 0},
			want: true,
		},
		{
			name: "same major/minor lower patch",
			v1:   ProtocolVersion{1, 6, 0},
			v2:   ProtocolVersion{1, 6, 1},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v1.IsNewerThan(tt.v2); got != tt.want {
				t.Errorf("%v.IsNewerThan(%v) = %v, want %v", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}
