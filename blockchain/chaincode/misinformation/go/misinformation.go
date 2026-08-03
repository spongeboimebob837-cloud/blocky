// Misinformation monitoring chaincode.
//
// Consortium review workflow: multiple fact-checking organisations register
// on-chain, submit reports, and every stakeholder organisation votes on the
// report's legitimacy. Once >= 2/3 of registered orgs have voted, the report
// is finalised and becomes an immutable ledger record.
//
// Statuses: PENDING -> (votes) -> FINAL (accepted) | REJECTED.
// Deliberately simple, modular, platform-agnostic audit layer (per proposal).
// See DATA_MODEL.md.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hyperledger/fabric-contract-api-go/v2/contractapi"
)

const (
	statusPending  = "PENDING"
	statusFinal    = "FINAL"
	statusRejected = "REJECTED"
)

// Vote is one organisation's verdict on a report.
type Vote struct {
	VoterMSP string `json:"voter_msp"`
	Verdict  string `json:"verdict"` // "accept" | "reject"
	TxID     string `json:"txid"`
}

// ReportRecord is the on-chain audit record for one report.
// Raw text is never stored; only an integrity hash of it.
type ReportRecord struct {
	RowID         string   `json:"row_id"`
	Language      string   `json:"language"`
	ContentHash   string   `json:"content_hash"`
	ProposedLabel string   `json:"proposed_label"` // "0" reliable | "1" misinformation (claim)
	Confidence    float64  `json:"confidence"`
	ModelVersion  string   `json:"model_version"`
	Timestamp     string   `json:"timestamp"`
	SubmittedBy   string   `json:"submitted_by"`
	Status        string   `json:"status"` // PENDING | FINAL | REJECTED
	Votes         []Vote   `json:"votes"`
	FinalizedBy   string   `json:"finalized_by,omitempty" metadata:",optional"`
	FinalizedAt   string   `json:"finalized_at,omitempty" metadata:",optional"`
}

// RegisteredOrg is a stakeholder organisation enrolled on-chain.
type RegisteredOrg struct {
	MSPID        string `json:"mspid"`
	RegisteredAt string `json:"registered_at"`
}

// MisinformationContract provides the chaincode functions.
type MisinformationContract struct {
	contractapi.Contract
}

// newReportKey returns the composite ledger key for (language, row_id).
func newReportKey(ctx contractapi.TransactionContextInterface, language, rowID string) (string, error) {
	return ctx.GetStub().CreateCompositeKey("pred", []string{language, rowID})
}

// newOrgKey returns the composite ledger key for an enrolled organisation.
func newOrgKey(ctx contractapi.TransactionContextInterface, mspid string) (string, error) {
	return ctx.GetStub().CreateCompositeKey("org", []string{mspid})
}

// newVoteKey returns the composite ledger key for one org's vote on a report.
func newVoteKey(ctx contractapi.TransactionContextInterface, language, rowID, mspid string) (string, error) {
	return ctx.GetStub().CreateCompositeKey("vote", []string{language, rowID, mspid})
}

// validateReportInput normalises and checks a report's core fields.
func validateReportInput(rowID, language, contentHash, label, modelVersion, timestamp string, confidence float64) error {
	if strings.TrimSpace(rowID) == "" {
		return fmt.Errorf("row_id must not be empty")
	}
	if language != "nso" && language != "zul" && language != "eng" {
		return fmt.Errorf("language must be one of nso/zul/eng, got %q", language)
	}
	if label != "0" && label != "1" {
		return fmt.Errorf("label must be \"0\" or \"1\", got %q", label)
	}
	if confidence < 0 || confidence > 1 {
		return fmt.Errorf("confidence must be in [0,1], got %f", confidence)
	}
	if strings.TrimSpace(modelVersion) == "" {
		return fmt.Errorf("model_version must not be empty")
	}
	if len(contentHash) != 64 {
		return fmt.Errorf("content_hash must be a 64-char sha256 hex digest, got %d chars", len(contentHash))
	}
	if _, err := hex.DecodeString(contentHash); err != nil {
		return fmt.Errorf("content_hash is not valid hex: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, timestamp); err != nil {
		return fmt.Errorf("timestamp must be RFC3339 UTC, got %q: %v", timestamp, err)
	}
	return nil
}

// getRegisteredOrgs returns all enrolled stakeholder organisations.
func (c *MisinformationContract) getRegisteredOrgs(
	ctx contractapi.TransactionContextInterface,
) ([]*RegisteredOrg, error) {
	iter, err := ctx.GetStub().GetStateByPartialCompositeKey("org", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open org query: %v", err)
	}
	defer iter.Close()
	var orgs []*RegisteredOrg
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to advance org iterator: %v", err)
		}
		var org RegisteredOrg
		if err := json.Unmarshal(kv.Value, &org); err != nil {
			return nil, fmt.Errorf("failed to unmarshal org %q: %v", kv.Key, err)
		}
		orgs = append(orgs, &org)
	}
	return orgs, nil
}

// isRegisteredOrg reports whether the caller's MSP is an enrolled stakeholder org.
func (c *MisinformationContract) isRegisteredOrg(
	ctx contractapi.TransactionContextInterface, mspid string,
) (bool, error) {
	key, err := newOrgKey(ctx, mspid)
	if err != nil {
		return false, err
	}
	state, err := ctx.GetStub().GetState(key)
	if err != nil {
		return false, fmt.Errorf("failed to read org state: %v", err)
	}
	return state != nil, nil
}

// quorumFor returns the 2/3-of-registered-orgs threshold, rounding up.
func quorumFor(registeredCount int) int {
	if registeredCount < 1 {
		return 0
	}
	return (2*registeredCount + 2) / 3
}

// RegisterOrg enrolls the calling organisation as a stakeholder.
// Idempotent: re-registering is a no-op. Returns the registered MSPID.
func (c *MisinformationContract) RegisterOrg(
	ctx contractapi.TransactionContextInterface,
) (string, error) {
	mspid, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return "", fmt.Errorf("failed to read caller MSP: %v", err)
	}
	key, err := newOrgKey(ctx, mspid)
	if err != nil {
		return "", fmt.Errorf("failed to build org key: %v", err)
	}
	exists, err := ctx.GetStub().GetState(key)
	if err != nil {
		return "", fmt.Errorf("failed to read org state: %v", err)
	}
	if exists != nil {
		return mspid, nil
	}
	org := RegisteredOrg{
		MSPID:        mspid,
		RegisteredAt: time.Now().UTC().Format(time.RFC3339),
	}
	orgBytes, err := json.Marshal(org)
	if err != nil {
		return "", fmt.Errorf("failed to marshal org: %v", err)
	}
	if err := ctx.GetStub().PutState(key, orgBytes); err != nil {
		return "", fmt.Errorf("failed to write org: %v", err)
	}
	return mspid, nil
}

// ListRegisteredOrgs returns all enrolled stakeholder organisations.
func (c *MisinformationContract) ListRegisteredOrgs(
	ctx contractapi.TransactionContextInterface,
) ([]*RegisteredOrg, error) {
	return c.getRegisteredOrgs(ctx)
}

// SubmitReport creates a PENDING report awaiting stakeholder votes.
// Only a registered organisation may submit.
func (c *MisinformationContract) SubmitReport(
	ctx contractapi.TransactionContextInterface,
	rowID, language, contentHash, label string,
	confidence float64,
	modelVersion, timestamp string,
) error {
	if err := validateReportInput(rowID, language, contentHash, label, modelVersion, timestamp, confidence); err != nil {
		return err
	}

	submittedBy, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return fmt.Errorf("failed to read caller MSP: %v", err)
	}
	if ok, err := c.isRegisteredOrg(ctx, submittedBy); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("org %s is not a registered stakeholder; call RegisterOrg first", submittedBy)
	}

	key, err := newReportKey(ctx, language, rowID)
	if err != nil {
		return fmt.Errorf("failed to build key: %v", err)
	}
	if exists, _ := ctx.GetStub().GetState(key); exists != nil {
		return fmt.Errorf("report for %s/%s already exists (immutable once finalised)", language, rowID)
	}

	record := ReportRecord{
		RowID:         rowID,
		Language:      language,
		ContentHash:   contentHash,
		ProposedLabel: label,
		Confidence:    confidence,
		ModelVersion:  modelVersion,
		Timestamp:     timestamp,
		SubmittedBy:   submittedBy,
		Status:        statusPending,
		Votes:         []Vote{},
	}
	recordBytes, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %v", err)
	}
	if err := ctx.GetStub().PutState(key, recordBytes); err != nil {
		return fmt.Errorf("failed to write record: %v", err)
	}
	return nil
}

// CastVote records one organisation's verdict on a PENDING report.
// Each org votes once. Verdict is "accept" or "reject".
func (c *MisinformationContract) CastVote(
	ctx contractapi.TransactionContextInterface,
	language, rowID, verdict string,
) error {
	if verdict != "accept" && verdict != "reject" {
		return fmt.Errorf("verdict must be \"accept\" or \"reject\", got %q", verdict)
	}

	voterMSP, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return fmt.Errorf("failed to read caller MSP: %v", err)
	}
	if ok, err := c.isRegisteredOrg(ctx, voterMSP); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("org %s is not a registered stakeholder; call RegisterOrg first", voterMSP)
	}

	key, err := newReportKey(ctx, language, rowID)
	if err != nil {
		return fmt.Errorf("failed to build key: %v", err)
	}
	recordBytes, err := ctx.GetStub().GetState(key)
	if err != nil {
		return fmt.Errorf("failed to read state: %v", err)
	}
	if recordBytes == nil {
		return fmt.Errorf("no report found for %s/%s", language, rowID)
	}
	var record ReportRecord
	if err := json.Unmarshal(recordBytes, &record); err != nil {
		return fmt.Errorf("failed to unmarshal record: %v", err)
	}
	if record.Status != statusPending {
		return fmt.Errorf("report %s/%s is %s; votes are closed", language, rowID, record.Status)
	}

	voteKey, err := newVoteKey(ctx, language, rowID, voterMSP)
	if err != nil {
		return fmt.Errorf("failed to build vote key: %v", err)
	}
	if exists, _ := ctx.GetStub().GetState(voteKey); exists != nil {
		return fmt.Errorf("org %s already voted on %s/%s", voterMSP, language, rowID)
	}

	txID := ctx.GetStub().GetTxID()
	vote := Vote{VoterMSP: voterMSP, Verdict: verdict, TxID: txID}
	voteBytes, err := json.Marshal(vote)
	if err != nil {
		return fmt.Errorf("failed to marshal vote: %v", err)
	}
	if err := ctx.GetStub().PutState(voteKey, voteBytes); err != nil {
		return fmt.Errorf("failed to write vote: %v", err)
	}

	record.Votes = append(record.Votes, vote)
	recordBytes, err = json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %v", err)
	}
	if err := ctx.GetStub().PutState(key, recordBytes); err != nil {
		return fmt.Errorf("failed to update record: %v", err)
	}
	return nil
}

// FinalizeReport locks a PENDING report into a FINAL or REJECTED record once
// >= 2/3 of registered orgs have voted. After this the record is immutable.
func (c *MisinformationContract) FinalizeReport(
	ctx contractapi.TransactionContextInterface,
	language, rowID string,
) error {
	finalizerMSP, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return fmt.Errorf("failed to read caller MSP: %v", err)
	}
	if ok, err := c.isRegisteredOrg(ctx, finalizerMSP); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("org %s is not a registered stakeholder; call RegisterOrg first", finalizerMSP)
	}

	key, err := newReportKey(ctx, language, rowID)
	if err != nil {
		return fmt.Errorf("failed to build key: %v", err)
	}
	recordBytes, err := ctx.GetStub().GetState(key)
	if err != nil {
		return fmt.Errorf("failed to read state: %v", err)
	}
	if recordBytes == nil {
		return fmt.Errorf("no report found for %s/%s", language, rowID)
	}
	var record ReportRecord
	if err := json.Unmarshal(recordBytes, &record); err != nil {
		return fmt.Errorf("failed to unmarshal record: %v", err)
	}
	if record.Status != statusPending {
		return fmt.Errorf("report %s/%s is already %s", language, rowID, record.Status)
	}

	orgs, err := c.getRegisteredOrgs(ctx)
	if err != nil {
		return err
	}
	quorum := quorumFor(len(orgs))
	if quorum < 1 {
		return fmt.Errorf("no registered stakeholder orgs; cannot finalise")
	}
	if len(record.Votes) < quorum {
		return fmt.Errorf("only %d of %d registered orgs have voted; %d votes required for 2/3 quorum",
			len(record.Votes), len(orgs), quorum)
	}

	accept := 0
	for _, v := range record.Votes {
		if v.Verdict == "accept" {
			accept++
		}
	}
	record.FinalizedBy = finalizerMSP
	record.FinalizedAt = time.Now().UTC().Format(time.RFC3339)
	if accept >= quorum {
		record.Status = statusFinal
	} else {
		record.Status = statusRejected
	}

	recordBytes, err = json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %v", err)
	}
	if err := ctx.GetStub().PutState(key, recordBytes); err != nil {
		return fmt.Errorf("failed to write final record: %v", err)
	}
	return nil
}

// QueryReport returns the record for a (language, row_id).
func (c *MisinformationContract) QueryReport(
	ctx contractapi.TransactionContextInterface,
	language, rowID string,
) (*ReportRecord, error) {
	key, err := newReportKey(ctx, language, rowID)
	if err != nil {
		return nil, fmt.Errorf("failed to build key: %v", err)
	}
	recordBytes, err := ctx.GetStub().GetState(key)
	if err != nil {
		return nil, fmt.Errorf("failed to read state: %v", err)
	}
	if recordBytes == nil {
		return nil, fmt.Errorf("no report found for %s/%s", language, rowID)
	}
	var record ReportRecord
	if err := json.Unmarshal(recordBytes, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal record: %v", err)
	}
	return &record, nil
}

// QueryAllReports returns every report on the ledger.
func (c *MisinformationContract) QueryAllReports(
	ctx contractapi.TransactionContextInterface,
) ([]*ReportRecord, error) {
	resultsIter, err := ctx.GetStub().GetStateByPartialCompositeKey("pred", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open composite key query: %v", err)
	}
	defer resultsIter.Close()

	var records []*ReportRecord
	for resultsIter.HasNext() {
		kv, err := resultsIter.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to advance results iterator: %v", err)
		}
		var record ReportRecord
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			return nil, fmt.Errorf("failed to unmarshal record %q: %v", kv.Key, err)
		}
		records = append(records, &record)
	}
	return records, nil
}

// GetReportCount returns the number of reports on the ledger.
func (c *MisinformationContract) GetReportCount(
	ctx contractapi.TransactionContextInterface,
) (int, error) {
	resultsIter, err := ctx.GetStub().GetStateByPartialCompositeKey("pred", nil)
	if err != nil {
		return 0, fmt.Errorf("failed to open composite key query: %v", err)
	}
	defer resultsIter.Close()
	count := 0
	for resultsIter.HasNext() {
		if _, err := resultsIter.Next(); err != nil {
			return 0, fmt.Errorf("failed to advance results iterator: %v", err)
		}
		count++
	}
	return count, nil
}

// QueryVotes returns all votes cast on a report.
func (c *MisinformationContract) QueryVotes(
	ctx contractapi.TransactionContextInterface,
	language, rowID string,
) ([]*Vote, error) {
	iter, err := ctx.GetStub().GetStateByPartialCompositeKey("vote", []string{language, rowID})
	if err != nil {
		return nil, fmt.Errorf("failed to open vote query: %v", err)
	}
	defer iter.Close()
	var votes []*Vote
	for iter.HasNext() {
		kv, err := iter.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to advance vote iterator: %v", err)
		}
		var vote Vote
		if err := json.Unmarshal(kv.Value, &vote); err != nil {
			return nil, fmt.Errorf("failed to unmarshal vote %q: %v", kv.Key, err)
		}
		votes = append(votes, &vote)
	}
	sort.Slice(votes, func(i, j int) bool { return votes[i].VoterMSP < votes[j].VoterMSP })
	return votes, nil
}

// QueryReportHistory returns the full ledger history (prior values) of a key,
// proving the tamper-evident audit trail for (language, row_id).
func (c *MisinformationContract) QueryReportHistory(
	ctx contractapi.TransactionContextInterface,
	language, rowID string,
) ([]string, error) {
	key, err := newReportKey(ctx, language, rowID)
	if err != nil {
		return nil, fmt.Errorf("failed to build key: %v", err)
	}
	historyIter, err := ctx.GetStub().GetHistoryForKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to open history query: %v", err)
	}
	defer historyIter.Close()

	var entries []string
	for historyIter.HasNext() {
		entry, err := historyIter.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to advance history iterator: %v", err)
		}
		entries = append(entries, fmt.Sprintf(
			"tx=%s value=%s deleted=%v",
			entry.TxId, string(entry.Value), entry.IsDelete,
		))
	}
	return entries, nil
}

// ComputeContentHash is a convenience helper mirroring the Python bridge's
// sha256 hexdigest so report hashes can be verified on-chain.
func (c *MisinformationContract) ComputeContentHash(
	ctx contractapi.TransactionContextInterface,
	text string,
) (string, error) {
	if text == "" {
		return "", fmt.Errorf("text must not be empty")
	}
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:]), nil
}

func main() {
	chaincode, err := contractapi.NewChaincode(&MisinformationContract{})
	if err != nil {
		fmt.Printf("Error creating misinformation chaincode: %v\n", err)
		panic(err)
	}
	if err := chaincode.Start(); err != nil {
		fmt.Printf("Error starting misinformation chaincode: %v\n", err)
		panic(err)
	}
}
