package main

import (
	"encoding/json"
	"testing"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
)

const validHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const validTs = "2025-01-01T00:00:00Z"

func newTestStub(t *testing.T) *MockStub {
	t.Helper()
	cc, err := contractapi.NewChaincode(&MisinformationContract{})
	if err != nil {
		t.Fatalf("failed to create chaincode: %v", err)
	}
	stub := &MockStub{
		cc:          cc,
		State:       map[string][]byte{},
		TxID:        "tx1",
		TxTimestamp: nil,
	}
	stub.args = [][]byte{}
	stub.signedProposal = &peer.SignedProposal{}
	return stub
}

// invokeAs sets the mock invoker MSP for the next call.
func invokeAs(stub *MockStub, txID, msp string, args ...string) *peer.Response {
	stub.TxID = txID
	stub.CreatorMSP = msp
	call := [][]byte{[]byte(args[0])}
	for _, a := range args[1:] {
		call = append(call, []byte(a))
	}
	return stub.MockInvoke(txID, call)
}

func registerOrgs(t *testing.T, stub *MockStub, msps ...string) {
	t.Helper()
	for _, msp := range msps {
		if res := invokeAs(stub, "tx-reg", msp, "RegisterOrg"); res.Status != 200 {
			t.Fatalf("RegisterOrg for %s failed: %s", msp, res.Message)
		}
	}
}

func TestRegisterOrgAndList(t *testing.T) {
	stub := newTestStub(t)
	registerOrgs(t, stub, "Org1MSP", "Org2MSP", "Org3MSP")

	res := invokeAs(stub, "tx-list", "Org1MSP", "ListRegisteredOrgs")
	if res.Status != 200 {
		t.Fatalf("ListRegisteredOrgs failed: %s", res.Message)
	}
	var orgs []*RegisteredOrg
	if err := json.Unmarshal(res.Payload, &orgs); err != nil {
		t.Fatalf("bad payload: %v", err)
	}
	if len(orgs) != 3 {
		t.Fatalf("expected 3 registered orgs, got %d", len(orgs))
	}
}

func TestUnregisteredCannotSubmit(t *testing.T) {
	stub := newTestStub(t)
	res := invokeAs(stub, "tx1", "Org5MSP",
		"SubmitReport", "42", "nso", validHash, "1", "0.97", "model-v1", validTs)
	if res.Status == 200 {
		t.Fatalf("unregistered org should not be able to submit")
	}
}

func TestSubmitAndQuery(t *testing.T) {
	stub := newTestStub(t)
	registerOrgs(t, stub, "Org1MSP", "Org2MSP", "Org3MSP")

	res := invokeAs(stub, "tx1", "Org1MSP",
		"SubmitReport", "42", "nso", validHash, "1", "0.97", "afroxlmr-large-nso-v1.0", validTs)
	if res.Status != 200 {
		t.Fatalf("SubmitReport failed: %s", res.Message)
	}

	res = invokeAs(stub, "tx2", "Org1MSP", "QueryReport", "nso", "42")
	if res.Status != 200 {
		t.Fatalf("QueryReport failed: %s", res.Message)
	}
	var record ReportRecord
	if err := json.Unmarshal(res.Payload, &record); err != nil {
		t.Fatalf("bad payload: %v", err)
	}
	if record.RowID != "42" || record.ProposedLabel != "1" || record.Confidence != 0.97 {
		t.Fatalf("unexpected record: %+v", record)
	}
	if record.Status != statusPending {
		t.Fatalf("expected PENDING, got %s", record.Status)
	}
	if record.SubmittedBy != "Org1MSP" {
		t.Fatalf("expected submitted_by Org1MSP, got %s", record.SubmittedBy)
	}
}

func TestDuplicateRejected(t *testing.T) {
	stub := newTestStub(t)
	registerOrgs(t, stub, "Org1MSP")
	args := []string{"SubmitReport", "42", "nso", validHash, "1", "0.97", "model-v1", validTs}
	if res := invokeAs(stub, "tx1", "Org1MSP", args...); res.Status != 200 {
		t.Fatalf("first submit failed: %s", res.Message)
	}
	if res := invokeAs(stub, "tx2", "Org1MSP", args...); res.Status == 200 {
		t.Fatalf("duplicate submit should have been rejected")
	}
}

func TestInvalidInputsRejected(t *testing.T) {
	registerOrgs := func(stub *MockStub) {
		invokeAs(stub, "tx-reg", "Org1MSP", "RegisterOrg")
	}
	cases := [][]string{
		{"", "nso", validHash, "1", "0.5", "model", validTs},
		{"42", "fr", validHash, "1", "0.5", "model", validTs},
		{"42", "nso", validHash, "2", "0.5", "model", validTs},
		{"42", "nso", validHash, "1", "1.5", "model", validTs},
		{"42", "nso", "short", "1", "0.5", "model", validTs},
		{"42", "nso", validHash, "1", "0.5", "", validTs},
		{"42", "nso", validHash, "1", "0.5", "model", "not-a-date"},
	}
	for i, args := range cases {
		stub := newTestStub(t)
		registerOrgs(stub)
		res := invokeAs(stub, "tx1", "Org1MSP", append([]string{"SubmitReport"}, args...)...)
		if res.Status == 200 {
			t.Fatalf("case %d should have been rejected", i)
		}
	}
}

func TestVotingWorkflow(t *testing.T) {
	stub := newTestStub(t)
	registerOrgs(t, stub, "Org1MSP", "Org2MSP", "Org3MSP")

	if res := invokeAs(stub, "tx1", "Org1MSP",
		"SubmitReport", "42", "nso", validHash, "1", "0.97", "model-v1", validTs); res.Status != 200 {
		t.Fatalf("submit failed: %s", res.Message)
	}

	if res := invokeAs(stub, "tx2", "Org1MSP", "CastVote", "nso", "42", "accept"); res.Status != 200 {
		t.Fatalf("org1 vote failed: %s", res.Message)
	}

	// org1 cannot vote twice
	if res := invokeAs(stub, "tx3", "Org1MSP", "CastVote", "nso", "42", "reject"); res.Status == 200 {
		t.Fatalf("double vote should have been rejected")
	}

	// finalizing with only 1 of 3 votes must fail (quorum 2)
	if res := invokeAs(stub, "tx4", "Org2MSP", "FinalizeReport", "nso", "42"); res.Status == 200 {
		t.Fatalf("finalize before quorum should fail")
	}

	if res := invokeAs(stub, "tx5", "Org2MSP", "CastVote", "nso", "42", "accept"); res.Status != 200 {
		t.Fatalf("org2 vote failed: %s", res.Message)
	}

	// quorum reached: 2 of 3 accepted -> FINAL
	if res := invokeAs(stub, "tx6", "Org3MSP", "FinalizeReport", "nso", "42"); res.Status != 200 {
		t.Fatalf("finalize failed: %s", res.Message)
	}

	res := invokeAs(stub, "tx7", "Org3MSP", "QueryReport", "nso", "42")
	if res.Status != 200 {
		t.Fatalf("query failed: %s", res.Message)
	}
	var record ReportRecord
	if err := json.Unmarshal(res.Payload, &record); err != nil {
		t.Fatalf("bad payload: %v", err)
	}
	if record.Status != statusFinal {
		t.Fatalf("expected FINAL, got %s", record.Status)
	}
	if len(record.Votes) != 2 {
		t.Fatalf("expected 2 votes, got %d", len(record.Votes))
	}

	// votes are closed once FINAL
	if res := invokeAs(stub, "tx8", "Org3MSP", "CastVote", "nso", "42", "accept"); res.Status == 200 {
		t.Fatalf("vote after finalize should be rejected")
	}
}

func TestRejectionWorkflow(t *testing.T) {
	stub := newTestStub(t)
	registerOrgs(t, stub, "Org1MSP", "Org2MSP")

	if res := invokeAs(stub, "tx1", "Org1MSP",
		"SubmitReport", "7", "zul", validHash, "0", "0.6", "model-v1", validTs); res.Status != 200 {
		t.Fatalf("submit failed: %s", res.Message)
	}
	if res := invokeAs(stub, "tx2", "Org1MSP", "CastVote", "zul", "7", "reject"); res.Status != 200 {
		t.Fatalf("org1 vote failed: %s", res.Message)
	}
	if res := invokeAs(stub, "tx3", "Org2MSP", "CastVote", "zul", "7", "reject"); res.Status != 200 {
		t.Fatalf("org2 vote failed: %s", res.Message)
	}
	// both orgs rejected -> REJECTED
	if res := invokeAs(stub, "tx4", "Org1MSP", "FinalizeReport", "zul", "7"); res.Status != 200 {
		t.Fatalf("finalize failed: %s", res.Message)
	}
	res := invokeAs(stub, "tx5", "Org1MSP", "QueryReport", "zul", "7")
	var record ReportRecord
	if err := json.Unmarshal(res.Payload, &record); err != nil {
		t.Fatalf("bad payload: %v", err)
	}
	if record.Status != statusRejected {
		t.Fatalf("expected REJECTED, got %s", record.Status)
	}
}

func TestQueryVotesAndHistory(t *testing.T) {
	stub := newTestStub(t)
	registerOrgs(t, stub, "Org1MSP", "Org2MSP")
	invokeAs(stub, "tx1", "Org1MSP",
		"SubmitReport", "1", "nso", validHash, "1", "0.9", "model-v1", validTs)
	invokeAs(stub, "tx2", "Org1MSP", "CastVote", "nso", "1", "accept")
	invokeAs(stub, "tx3", "Org2MSP", "CastVote", "nso", "1", "reject")

	res := invokeAs(stub, "tx4", "Org1MSP", "QueryVotes", "nso", "1")
	if res.Status != 200 {
		t.Fatalf("QueryVotes failed: %s", res.Message)
	}
	var votes []*Vote
	if err := json.Unmarshal(res.Payload, &votes); err != nil {
		t.Fatalf("bad payload: %v", err)
	}
	if len(votes) != 2 {
		t.Fatalf("expected 2 votes, got %d", len(votes))
	}

	res = invokeAs(stub, "tx5", "Org1MSP", "QueryReportHistory", "nso", "1")
	if res.Status != 200 || len(res.Payload) == 0 {
		t.Fatalf("expected history entries: %s", res.Message)
	}
}

func TestCountAndAll(t *testing.T) {
	stub := newTestStub(t)
	registerOrgs(t, stub, "Org1MSP")
	invokeAs(stub, "tx1", "Org1MSP", "SubmitReport", "1", "nso", validHash, "0", "0.80", "model-v1", validTs)
	invokeAs(stub, "tx2", "Org1MSP", "SubmitReport", "2", "zul", validHash, "1", "0.90", "model-v1", validTs)

	res := invokeAs(stub, "tx3", "Org1MSP", "GetReportCount")
	if res.Status != 200 || string(res.Payload) != "2" {
		t.Fatalf("expected count 2, got %s (%s)", res.Payload, res.Message)
	}

	res = invokeAs(stub, "tx4", "Org1MSP", "QueryAllReports")
	if res.Status != 200 {
		t.Fatalf("QueryAll failed: %s", res.Message)
	}
	var records []*ReportRecord
	if err := json.Unmarshal(res.Payload, &records); err != nil {
		t.Fatalf("bad payload: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
}

func TestComputeContentHash(t *testing.T) {
	stub := newTestStub(t)
	res := invokeAs(stub, "tx1", "Org1MSP", "ComputeContentHash", "hello misinformation")
	if res.Status != 200 || len(res.Payload) != 64 {
		t.Fatalf("unexpected hash result: %s", res.Payload)
	}
}
