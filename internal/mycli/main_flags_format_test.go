package mycli

import (
	"testing"
)

func TestValidateSpannerOptions_FormatFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    *spannerOptions
		wantErr bool
		errMsg  string
	}{
		{
			name: "no format flags",
			opts: &spannerOptions{
				ProjectId:  "test",
				InstanceId: "test",
				DatabaseId: "test",
			},
			wantErr: false,
		},
		{
			name: "only --table",
			opts: &spannerOptions{
				ProjectId:  "test",
				InstanceId: "test",
				DatabaseId: "test",
				Table:      true,
			},
			wantErr: false,
		},
		{
			name: "only --html",
			opts: &spannerOptions{
				ProjectId:  "test",
				InstanceId: "test",
				DatabaseId: "test",
				HTML:       true,
			},
			wantErr: false,
		},
		{
			name: "only --xml",
			opts: &spannerOptions{
				ProjectId:  "test",
				InstanceId: "test",
				DatabaseId: "test",
				XML:        true,
			},
			wantErr: false,
		},
		{
			name: "only --csv",
			opts: &spannerOptions{
				ProjectId:  "test",
				InstanceId: "test",
				DatabaseId: "test",
				CSV:        true,
			},
			wantErr: false,
		},
		{
			name: "only --format=table",
			opts: &spannerOptions{
				ProjectId:  "test",
				InstanceId: "test",
				DatabaseId: "test",
				Format:     "table",
			},
			wantErr: false,
		},
		{
			name: "only --format=csv",
			opts: &spannerOptions{
				ProjectId:  "test",
				InstanceId: "test",
				DatabaseId: "test",
				Format:     "csv",
			},
			wantErr: false,
		},
		{
			name: "--table and --html are mutually exclusive",
			opts: &spannerOptions{
				ProjectId:  "test",
				InstanceId: "test",
				DatabaseId: "test",
				Table:      true,
				HTML:       true,
			},
			wantErr: true,
			errMsg:  "invalid combination: --table, --html, --xml, --csv, and --format are mutually exclusive",
		},
		{
			name: "--table and --xml are mutually exclusive",
			opts: &spannerOptions{
				ProjectId:  "test",
				InstanceId: "test",
				DatabaseId: "test",
				Table:      true,
				XML:        true,
			},
			wantErr: true,
			errMsg:  "invalid combination: --table, --html, --xml, --csv, and --format are mutually exclusive",
		},
		{
			name: "--html and --xml are mutually exclusive",
			opts: &spannerOptions{
				ProjectId:  "test",
				InstanceId: "test",
				DatabaseId: "test",
				HTML:       true,
				XML:        true,
			},
			wantErr: true,
			errMsg:  "invalid combination: --table, --html, --xml, --csv, and --format are mutually exclusive",
		},
		{
			name: "--table and --csv are mutually exclusive",
			opts: &spannerOptions{
				ProjectId:  "test",
				InstanceId: "test",
				DatabaseId: "test",
				Table:      true,
				CSV:        true,
			},
			wantErr: true,
			errMsg:  "invalid combination: --table, --html, --xml, --csv, and --format are mutually exclusive",
		},
		{
			name: "--html and --csv are mutually exclusive",
			opts: &spannerOptions{
				ProjectId:  "test",
				InstanceId: "test",
				DatabaseId: "test",
				HTML:       true,
				CSV:        true,
			},
			wantErr: true,
			errMsg:  "invalid combination: --table, --html, --xml, --csv, and --format are mutually exclusive",
		},
		{
			name: "--xml and --csv are mutually exclusive",
			opts: &spannerOptions{
				ProjectId:  "test",
				InstanceId: "test",
				DatabaseId: "test",
				XML:        true,
				CSV:        true,
			},
			wantErr: true,
			errMsg:  "invalid combination: --table, --html, --xml, --csv, and --format are mutually exclusive",
		},
		{
			name: "--format and --table are mutually exclusive",
			opts: &spannerOptions{
				ProjectId:  "test",
				InstanceId: "test",
				DatabaseId: "test",
				Format:     "csv",
				Table:      true,
			},
			wantErr: true,
			errMsg:  "invalid combination: --table, --html, --xml, --csv, and --format are mutually exclusive",
		},
		{
			name: "--format and --html are mutually exclusive",
			opts: &spannerOptions{
				ProjectId:  "test",
				InstanceId: "test",
				DatabaseId: "test",
				Format:     "table",
				HTML:       true,
			},
			wantErr: true,
			errMsg:  "invalid combination: --table, --html, --xml, --csv, and --format are mutually exclusive",
		},
		{
			name: "all three format flags are mutually exclusive",
			opts: &spannerOptions{
				ProjectId:  "test",
				InstanceId: "test",
				DatabaseId: "test",
				Table:      true,
				HTML:       true,
				XML:        true,
			},
			wantErr: true,
			errMsg:  "invalid combination: --table, --html, --xml, --csv, and --format are mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSpannerOptions(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSpannerOptions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("ValidateSpannerOptions() error = %v, wantErrMsg %v", err.Error(), tt.errMsg)
			}
		})
	}
}
