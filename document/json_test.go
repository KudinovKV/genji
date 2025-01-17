package document_test

import (
	"testing"

	"github.com/genjidb/genji/document"
	"github.com/genjidb/genji/types"
	"github.com/stretchr/testify/require"
)

func TestValueString(t *testing.T) {
	tests := []struct {
		name     string
		value    types.Value
		expected string
	}{
		{"null", types.NewNullValue(), "NULL"},
		{"blob", types.NewBlobValue([]byte("bar")), `"YmFy"`},
		{"string", types.NewTextValue("bar"), `'bar'`},
		{"bool", types.NewBoolValue(true), "true"},
		{"int", types.NewIntegerValue(10), "10"},
		{"double", types.NewDoubleValue(10.1), "10.1"},
		{"double with no decimal", types.NewDoubleValue(10), "10"},
		{"big double", types.NewDoubleValue(1e15), "1e+15"},
		{"document", types.NewDocumentValue(document.NewFieldBuffer().Add("a", types.NewIntegerValue(10))), "{\"a\": 10}"},
		{"array", types.NewArrayValue(document.NewValueBuffer(types.NewIntegerValue(10))), "[10]"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, document.ValueToString(test.value))
		})
	}
}
