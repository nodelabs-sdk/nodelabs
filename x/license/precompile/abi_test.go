package licenseprecompile

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// licenseTuple is the ABI shape of the License struct, mirroring LicenseOutput.
const licenseTuple = "(uint64,string,address,string,string,string)"

// licenseTypeTuple is the ABI shape of the LicenseType struct, mirroring
// LicenseTypeOutput.
const licenseTypeTuple = "(string,bool,uint256,uint256,uint256,uint256)"

// wantMethodSigs pins the full canonical signature of every ABI method, not
// just its name. abi.json is hand-maintained alongside LicenseI.sol with no
// codegen between them, so a name-only check would pass even if abi.json were
// never updated — the drift this is here to catch. go-ethereum derives the
// 4-byte selector from Sig, so pinning Sig pins the selector too.
var wantMethodSigs = map[string]string{
	// transactions
	CreateLicenseTypeMethod: "createLicenseType(string,bool,uint256)",
	UpdateLicenseTypeMethod: "updateLicenseType(string,bool)",
	IssueLicensesMethod:     "issueLicenses((string,address,string,string,uint64)[])",
	RevokeLicensesMethod:    "revokeLicenses(string,uint64[])",
	// queries
	LicenseTypeMethod:             "licenseType(string)",
	LicenseTypesMethod:            "licenseTypes()",
	LicenseMethod:                 "license(uint64)",
	LicensesMethod:                "licenses()",
	LicensesByTypeMethod:          "licensesByType(string)",
	LicensesByHolderMethod:        "licensesByHolder(address)",
	LicensesByHolderAndTypeMethod: "licensesByHolderAndType(address,string)",
}

// wantMethodOutputs pins return shapes, which Sig does not cover. These are
// what licenseToOutput / licenseTypeToOutput must keep packing into.
var wantMethodOutputs = map[string][]string{
	CreateLicenseTypeMethod:       {"bool"},
	UpdateLicenseTypeMethod:       {"bool"},
	IssueLicensesMethod:           {"uint64[]"},
	RevokeLicensesMethod:          {"uint64[]"},
	LicenseTypeMethod:             {licenseTypeTuple},
	LicenseTypesMethod:            {licenseTypeTuple + "[]"},
	LicenseMethod:                 {licenseTuple},
	LicensesMethod:                {licenseTuple + "[]"},
	LicensesByTypeMethod:          {licenseTuple + "[]"},
	LicensesByHolderMethod:        {licenseTuple + "[]"},
	LicensesByHolderAndTypeMethod: {licenseTuple + "[]"},
}

// wantEventSigs pins event signatures, and so topic0.
var wantEventSigs = map[string]string{
	EventTypeLicenseTypeCreated: "LicenseTypeCreated(string,bool,uint256)",
	EventTypeLicenseTypeUpdated: "LicenseTypeUpdated(string,bool)",
	EventTypeLicenseIssued:      "LicenseIssued(address,address,string,uint64)",
	EventTypeLicenseRevoked:     "LicenseRevoked(address,string,uint64[])",
}

// TestABISignatures asserts that every method and event in abi.json has
// exactly the expected signature and return shape, and that abi.json declares
// nothing beyond them. An extra method here is calldata the dispatcher does
// not handle; a changed signature is a broken client.
func TestABISignatures(t *testing.T) {
	for name, wantSig := range wantMethodSigs {
		m, ok := ABI.Methods[name]
		require.Truef(t, ok, "method %q missing from ABI", name)
		require.Equalf(t, wantSig, m.Sig, "signature drift for method %q", name)

		gotOutputs := make([]string, 0, len(m.Outputs))
		for _, o := range m.Outputs {
			gotOutputs = append(gotOutputs, o.Type.String())
		}
		require.Equalf(t, wantMethodOutputs[name], gotOutputs, "output drift for method %q", name)
	}
	require.Len(t, ABI.Methods, len(wantMethodSigs), "abi.json declares a method the dispatcher does not handle")

	for name, wantSig := range wantEventSigs {
		e, ok := ABI.Events[name]
		require.Truef(t, ok, "event %q missing from ABI", name)
		require.Equalf(t, wantSig, e.Sig, "signature drift for event %q", name)
	}
	require.Len(t, ABI.Events, len(wantEventSigs), "abi.json declares an unexpected event")
}

// TestIsTransaction asserts that write methods are classified as transactions
// and read methods are not. This is what gates state-changing calls in readonly mode.
func TestIsTransaction(t *testing.T) {
	p := Precompile{}

	txMethods := []string{
		CreateLicenseTypeMethod, UpdateLicenseTypeMethod,
		IssueLicensesMethod, RevokeLicensesMethod,
	}
	for _, name := range txMethods {
		m := ABI.Methods[name]
		require.Truef(t, p.IsTransaction(&m), "%s should be a transaction", name)
	}

	queryMethods := []string{
		LicenseTypeMethod, LicenseTypesMethod,
		LicenseMethod, LicensesMethod, LicensesByTypeMethod,
		LicensesByHolderMethod, LicensesByHolderAndTypeMethod,
	}
	for _, name := range queryMethods {
		m := ABI.Methods[name]
		require.Falsef(t, p.IsTransaction(&m), "%s should be a query, not a transaction", name)
	}
}
