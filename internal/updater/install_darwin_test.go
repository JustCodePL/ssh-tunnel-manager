//go:build darwin

package updater

import "testing"

func TestParseMountPoint(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "volume name with spaces",
			output: "/dev/disk4\tGUID_partition_scheme\n/dev/disk4s1\tApple_HFS\t/Volumes/SSH Tunnel Manager\n",
			want:   "/Volumes/SSH Tunnel Manager",
		},
		{
			name:   "APFS volume",
			output: "/dev/disk5s1         Apple_APFS                 /Volumes/SSH Tunnel Manager 2\n",
			want:   "/Volumes/SSH Tunnel Manager 2",
		},
		{
			name:   "no mounted volume",
			output: "/dev/disk4\tGUID_partition_scheme\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseMountPoint(tt.output); got != tt.want {
				t.Fatalf("parseMountPoint() = %q, want %q", got, tt.want)
			}
		})
	}
}
