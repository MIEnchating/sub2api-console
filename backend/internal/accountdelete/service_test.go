package accountdelete

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type deleteRepository struct {
	account         *business.AccountDetail
	accounts        map[string]*business.AccountDetail
	protection      business.AccountMutationProtection
	confirmErr      error
	deleteErr       error
	recordErr       error
	reconcileErr    error
	deleted         bool
	deletedIDs      []string
	keyReconciled   bool
	operation       business.AccountOperation
	confirmedScope  business.AccountDeleteScope
	reconciledScope business.AccountDeleteScope
	deletedScope    business.AccountDeleteScope
	audits          []business.AccountOperation
	accountReads    int
	afterFirstGet   func(*business.AccountDetail)
}

func (repository *deleteRepository) Mode(context.Context) (string, error) { return "完全模式", nil }

func (repository *deleteRepository) Account(_ context.Context, accountID string) (*business.AccountDetail, error) {
	repository.accountReads++
	account := repository.account
	if repository.accounts != nil {
		account = repository.accounts[accountID]
	}
	if account == nil {
		return nil, errors.New("missing")
	}
	copy := *account
	copy.Bindings = append([]business.AccountBinding{}, account.Bindings...)
	if repository.accountReads == 1 && repository.afterFirstGet != nil {
		repository.afterFirstGet(account)
	}
	return &copy, nil
}

func (repository *deleteRepository) AccountMutationProtection(context.Context, string) (business.AccountMutationProtection, error) {
	return repository.protection, nil
}

func (repository *deleteRepository) ConfirmAccountDeleteScope(
	_ context.Context,
	_ string,
	scope business.AccountDeleteScope,
) error {
	repository.confirmedScope = scope
	return repository.confirmErr
}

func (repository *deleteRepository) ReconcileDeletedUpstreamKeyProjection(
	_ context.Context,
	_ string,
	scope business.AccountDeleteScope,
) error {
	repository.keyReconciled = true
	repository.reconciledScope = scope
	return repository.reconcileErr
}

func (repository *deleteRepository) DeleteAccountProjectionWithScope(
	_ context.Context,
	accountID string,
	scope business.AccountDeleteScope,
	operation business.AccountOperation,
) error {
	repository.deleted = true
	repository.deletedIDs = append(repository.deletedIDs, accountID)
	repository.deletedScope = scope
	repository.operation = operation
	return repository.deleteErr
}

func (repository *deleteRepository) DeleteAccountProjection(
	_ context.Context,
	accountID string,
	operation business.AccountOperation,
) error {
	repository.deleted = true
	repository.deletedIDs = append(repository.deletedIDs, accountID)
	repository.operation = operation
	return repository.deleteErr
}

func (repository *deleteRepository) RecordAccountOperation(_ context.Context, operation business.AccountOperation) error {
	repository.audits = append(repository.audits, operation)
	return repository.recordErr
}

type deletePrivate struct {
	record            *configstore.AuthRecord
	target            configstore.TargetSettings
	secretDeleteErr   error
	deletedSecrets    [][2]string
	afterSecretDelete func()
}

func (private *deletePrivate) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return private.target, nil
}

func (private *deletePrivate) AuthRecord(context.Context, string) (*configstore.AuthRecord, error) {
	return private.record, nil
}

func (private *deletePrivate) DeleteUpstreamKeySecrets(_ context.Context, host, keyID string) error {
	private.deletedSecrets = append(private.deletedSecrets, [2]string{host, keyID})
	if private.afterSecretDelete != nil {
		private.afterSecretDelete()
	}
	return private.secretDeleteErr
}

type deleteKeys struct {
	keys                []business.UpstreamCatalogKey
	deleteErr           error
	commitOnDeleteError bool
	deleteCalls         []string
	listErrors          []error
	listCalls           int
	afterDelete         func()
}

func (keys *deleteKeys) ListKeys(context.Context, configstore.AuthRecord) ([]business.UpstreamCatalogKey, error) {
	call := keys.listCalls
	keys.listCalls++
	if call < len(keys.listErrors) && keys.listErrors[call] != nil {
		return nil, keys.listErrors[call]
	}
	return append([]business.UpstreamCatalogKey{}, keys.keys...), nil
}

func (keys *deleteKeys) DeleteKey(_ context.Context, _ configstore.AuthRecord, keyID string) error {
	keys.deleteCalls = append(keys.deleteCalls, keyID)
	if keys.deleteErr == nil || keys.commitOnDeleteError {
		keys.keys = nil
		if keys.afterDelete != nil {
			keys.afterDelete()
		}
	}
	return keys.deleteErr
}

type deleteAdmin struct {
	err    error
	errors map[string]error
	calls  []string
}

func (admin *deleteAdmin) DeleteAccountWithVerification(_ context.Context, accountID string, verification bool) (map[string]any, error) {
	if !verification {
		return nil, errors.New("verification disabled")
	}
	admin.calls = append(admin.calls, accountID)
	if err := admin.errors[accountID]; err != nil {
		return nil, err
	}
	return map[string]any{"confirmed_absent": true}, admin.err
}

func boundAccount() *business.AccountDetail {
	authHost := "auth.example.com"
	return &business.AccountDetail{
		AccountStatus: business.AccountStatus{ID: "37", Name: "price-key", Groups: []string{"special"}},
		Bindings: []business.AccountBinding{{
			ID: 91, LocalAccountID: "37", UpstreamID: "upstream-1", UpstreamHost: "https://upstream.example.com",
			UpstreamKeyID: "key-8", UpstreamKeyName: "price-key", SourceAuthHost: &authHost,
		}},
	}
}

func configuredService(repository *deleteRepository, keys *deleteKeys, admin *deleteAdmin) *Service {
	return configuredServiceWithPrivate(repository, keys, admin, configuredDeletePrivate())
}

func configuredDeletePrivate() *deletePrivate {
	return &deletePrivate{record: &configstore.AuthRecord{
		Host: "auth.example.com", BaseURL: "https://upstream.example.com", UpstreamType: "sub2api", AuthMode: "admin_key",
	}, target: configstore.TargetSettings{
		BaseURL: "https://management-a.example.com", AdminKey: "admin-key", TimeoutSeconds: 5,
	}}
}

func configuredServiceWithPrivate(
	repository *deleteRepository,
	keys *deleteKeys,
	admin *deleteAdmin,
	private *deletePrivate,
) *Service {
	service := New(repository, private, keys, nil)
	service.adminFactory = func(configstore.TargetSettings) (Admin, error) { return admin, nil }
	return service
}

type deleteTaskStore struct{ tasks []taskstore.Task }

func (store *deleteTaskStore) Save(_ context.Context, task taskstore.Task) error {
	store.tasks = append(store.tasks, task)
	return nil
}

type heldDeleteRunner struct{ run func(context.Context) }

func (runner *heldDeleteRunner) Go(run func(context.Context)) error {
	runner.run = run
	return nil
}

func unboundAccount(id, name string) *business.AccountDetail {
	return &business.AccountDetail{
		AccountStatus: business.AccountStatus{ID: id, Name: name, Groups: []string{"special"}},
	}
}

func TestPreviewBatchPreservesOrderAndSummarizesUpstreamKeys(t *testing.T) {
	bound := boundAccount()
	repository := &deleteRepository{accounts: map[string]*business.AccountDetail{
		"37": bound,
		"38": unboundAccount("38", "management-only"),
	}}
	service := configuredService(repository, &deleteKeys{}, &deleteAdmin{})

	preview, err := service.PreviewBatch(context.Background(), []string{"38", "37"})
	if err != nil {
		t.Fatal(err)
	}
	if preview.AccountCount != 2 || preview.UpstreamKeyCount != 1 ||
		preview.Accounts[0].AccountID != "38" || preview.Accounts[1].AccountID != "37" {
		t.Fatalf("unexpected batch preview: %#v", preview)
	}
	if _, err := service.PreviewBatch(context.Background(), []string{"37", "37"}); err == nil ||
		!strings.Contains(err.Error(), "重复账号 ID") {
		t.Fatalf("duplicate account IDs were accepted: %v", err)
	}
}

func TestBatchDeleteContinuesAfterOneAccountFails(t *testing.T) {
	repository := &deleteRepository{accounts: map[string]*business.AccountDetail{
		"37": unboundAccount("37", "first"),
		"38": unboundAccount("38", "second"),
	}}
	tasks := &deleteTaskStore{}
	runner := &heldDeleteRunner{}
	admin := &deleteAdmin{errors: map[string]error{
		"37": &adminclient.AccountStillReadableError{AccountID: "37"},
	}}
	service := New(repository, configuredDeletePrivate(), &deleteKeys{}, tasks)
	service.UseTaskRunner(runner)
	service.adminFactory = func(configstore.TargetSettings) (Admin, error) { return admin, nil }
	preview, err := service.PreviewBatch(context.Background(), []string{"37", "38"})
	if err != nil {
		t.Fatal(err)
	}
	confirmations := make([]Confirmation, 0, len(preview.Accounts))
	for _, item := range preview.Accounts {
		confirmations = append(confirmations, Confirmation{
			AccountID: item.AccountID, ManagementBaseURL: item.ManagementBaseURL, Binding: item.Binding,
		})
	}
	if _, err := service.EnqueueBatch(context.Background(), confirmations, "tester"); err != nil {
		t.Fatal(err)
	}
	if runner.run == nil {
		t.Fatal("batch delete worker was not queued")
	}
	runner.run(context.Background())
	final := tasks.tasks[len(tasks.tasks)-1]
	if final.Status != "partial" || final.Result["succeeded"] != 1 || final.Result["failed"] != 1 {
		t.Fatalf("unexpected final task: %#v", final)
	}
	if strings.Join(admin.calls, ",") != "37,38" || len(repository.deletedIDs) != 1 || repository.deletedIDs[0] != "38" {
		t.Fatalf("batch did not continue safely: admin=%#v deleted=%#v", admin.calls, repository.deletedIDs)
	}
}

func TestBatchDeleteResultReportsOnlyActualRemoteRequests(t *testing.T) {
	beforeWrite := batchDeleteResult(1, 0, 1, []map[string]any{{
		"status": "failed", "management_account_delete_requested": false,
	}})
	if beforeWrite["remote_write"] != false {
		t.Fatalf("preflight failure was reported as a remote write: %#v", beforeWrite)
	}

	afterWrite := batchDeleteResult(1, 0, 1, []map[string]any{{
		"status": "failed", "upstream_key_delete_requested": true,
	}})
	if afterWrite["remote_write"] != true {
		t.Fatalf("attempted deletion was not reported as a remote write: %#v", afterWrite)
	}
}

func TestDeleteRemovesExactKeyThenManagementAccountAndProjection(t *testing.T) {
	repository := &deleteRepository{account: boundAccount()}
	keys := &deleteKeys{keys: []business.UpstreamCatalogKey{{KeyID: "key-8", Name: "price-key"}}}
	admin := &deleteAdmin{}
	private := configuredDeletePrivate()
	service := configuredServiceWithPrivate(repository, keys, admin, private)
	preview, err := service.Preview(context.Background(), "37")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Delete(context.Background(), preview, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys.deleteCalls) != 1 || keys.deleteCalls[0] != "key-8" {
		t.Fatalf("wrong key deletion: %#v", keys.deleteCalls)
	}
	if len(admin.calls) != 1 || admin.calls[0] != "37" {
		t.Fatalf("wrong account deletion: %#v", admin.calls)
	}
	if !result.UpstreamKeyDeleted || !result.ManagementAccountDeleted || !result.LocalProjectionDeleted || !result.ReadbackConfirmed {
		t.Fatalf("incomplete result: %#v", result)
	}
	if !repository.deleted || repository.operation.ObjectID != "37" || repository.operation.OperationType != "account.delete" {
		t.Fatalf("projection/audit not committed: %#v", repository.operation)
	}
	if repository.confirmedScope.UpstreamID != "upstream-1" || repository.confirmedScope.UpstreamKeyID != "key-8" ||
		repository.confirmedScope.BindingID != 91 || repository.reconciledScope != repository.confirmedScope ||
		repository.deletedScope != repository.confirmedScope {
		t.Fatalf("stable delete scope was not preserved: confirmed=%#v reconciled=%#v deleted=%#v",
			repository.confirmedScope, repository.reconciledScope, repository.deletedScope)
	}
	if !repository.keyReconciled || len(private.deletedSecrets) != 1 ||
		private.deletedSecrets[0] != [2]string{"auth.example.com", "key-8"} {
		t.Fatalf("confirmed key was not reconciled before account cleanup: local=%v secrets=%#v",
			repository.keyReconciled, private.deletedSecrets)
	}
	after := repository.operation.After.(map[string]any)
	if after["local_projection_deleted"] != true {
		t.Fatalf("success audit did not report the committed local cleanup: %#v", after)
	}
}

func TestDeleteWithoutBindingRemovesManagementAccountAndProjectionOnly(t *testing.T) {
	account := boundAccount()
	account.Bindings = nil
	repository := &deleteRepository{account: account}
	keys := &deleteKeys{keys: []business.UpstreamCatalogKey{{KeyID: "key-8", Name: "unrelated"}}}
	admin := &deleteAdmin{}
	service := configuredService(repository, keys, admin)

	preview, err := service.Preview(context.Background(), "37")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Binding != nil {
		t.Fatalf("unbound preview exposed a binding: %#v", preview.Binding)
	}
	result, err := service.Delete(context.Background(), preview, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if keys.listCalls != 0 || len(keys.deleteCalls) != 0 {
		t.Fatalf("unbound deletion touched upstream keys: lists=%d deletes=%#v", keys.listCalls, keys.deleteCalls)
	}
	if len(admin.calls) != 1 || admin.calls[0] != "37" || !repository.deleted {
		t.Fatalf("management/local deletion missing: admin=%#v local=%v", admin.calls, repository.deleted)
	}
	if result.UpstreamKeyDeleteRequested || result.UpstreamKeyDeleted ||
		!result.ManagementAccountDeleted || !result.LocalProjectionDeleted || !result.ReadbackConfirmed {
		t.Fatalf("unexpected management-only result: %#v", result)
	}
}

func TestDeleteKeyFailurePreventsManagementAndLocalDeletion(t *testing.T) {
	repository := &deleteRepository{account: boundAccount()}
	keys := &deleteKeys{keys: []business.UpstreamCatalogKey{{KeyID: "key-8"}}, deleteErr: errors.New("denied")}
	admin := &deleteAdmin{}
	private := configuredDeletePrivate()
	service := configuredServiceWithPrivate(repository, keys, admin, private)
	preview, _ := service.Preview(context.Background(), "37")
	result, err := service.Delete(context.Background(), preview, "tester")
	if err == nil || !strings.Contains(err.Error(), "删除请求返回错误") || !strings.Contains(err.Error(), "删除后仍可读") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.UpstreamKeyDeleteRequested || result.UpstreamKeyDeleted || result.ReadbackConfirmed ||
		len(admin.calls) != 0 || repository.deleted || repository.keyReconciled || len(private.deletedSecrets) != 0 {
		t.Fatalf("later deletion ran after key failure: result=%#v admin=%#v local=%v", result, admin.calls, repository.deleted)
	}
	assertUnconfirmedKeyDeleteAudit(t, repository.audits, "删除后仍可读")
}

func TestDeleteKeyErrorWithAbsentReadbackContinuesAfterConfirmedTargetState(t *testing.T) {
	repository := &deleteRepository{account: boundAccount()}
	keys := &deleteKeys{
		keys:                []business.UpstreamCatalogKey{{KeyID: "key-8"}},
		deleteErr:           errors.New("connection reset after write"),
		commitOnDeleteError: true,
	}
	admin := &deleteAdmin{}
	private := configuredDeletePrivate()
	service := configuredServiceWithPrivate(repository, keys, admin, private)
	preview, _ := service.Preview(context.Background(), "37")

	result, err := service.Delete(context.Background(), preview, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if !result.UpstreamKeyDeleteRequested || !result.UpstreamKeyDeleted || !result.ReadbackConfirmed ||
		!result.UpstreamKeyProjectionDeleted || !result.UpstreamKeySecretDeleted ||
		!result.ManagementAccountDeleted || !result.LocalProjectionDeleted || len(admin.calls) != 1 ||
		!repository.deleted || !repository.keyReconciled || len(private.deletedSecrets) != 1 || len(repository.audits) != 0 {
		t.Fatalf("confirmed target state did not continue deletion: result=%#v admin=%#v local=%v audits=%#v",
			result, admin.calls, repository.deleted, repository.audits)
	}
}

func TestDeleteAuditsPostKeyDeleteReadbackFailureWithoutClaimingDeletion(t *testing.T) {
	repository := &deleteRepository{account: boundAccount()}
	keys := &deleteKeys{
		keys:       []business.UpstreamCatalogKey{{KeyID: "key-8"}},
		listErrors: []error{nil, errors.New("readback unavailable")},
	}
	admin := &deleteAdmin{}
	private := configuredDeletePrivate()
	service := configuredServiceWithPrivate(repository, keys, admin, private)
	preview, _ := service.Preview(context.Background(), "37")

	result, err := service.Delete(context.Background(), preview, "tester")
	if err == nil || !strings.Contains(err.Error(), "删除后读回失败") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.UpstreamKeyDeleteRequested || result.UpstreamKeyDeleted || result.ReadbackConfirmed ||
		len(admin.calls) != 0 || repository.deleted || repository.keyReconciled || len(private.deletedSecrets) != 0 {
		t.Fatalf("readback failure overstated deletion or continued: result=%#v admin=%#v local=%v",
			result, admin.calls, repository.deleted)
	}
	assertUnconfirmedKeyDeleteAudit(t, repository.audits, "删除后读回失败")
}

func TestDeleteKeyErrorAndReadbackFailureAuditUnknownOutcome(t *testing.T) {
	repository := &deleteRepository{account: boundAccount()}
	keys := &deleteKeys{
		keys:       []business.UpstreamCatalogKey{{KeyID: "key-8"}},
		deleteErr:  errors.New("connection reset after write"),
		listErrors: []error{nil, errors.New("readback unavailable")},
	}
	admin := &deleteAdmin{}
	private := configuredDeletePrivate()
	service := configuredServiceWithPrivate(repository, keys, admin, private)
	preview, _ := service.Preview(context.Background(), "37")

	result, err := service.Delete(context.Background(), preview, "tester")
	if err == nil || !strings.Contains(err.Error(), "connection reset after write") ||
		!strings.Contains(err.Error(), "readback unavailable") || !strings.Contains(err.Error(), "删除结果未知") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.UpstreamKeyDeleteRequested || result.UpstreamKeyDeleted || result.ReadbackConfirmed ||
		len(admin.calls) != 0 || repository.deleted || repository.keyReconciled || len(private.deletedSecrets) != 0 {
		t.Fatalf("unknown key outcome was overstated or continued: result=%#v admin=%#v local=%v",
			result, admin.calls, repository.deleted)
	}
	assertUnconfirmedKeyDeleteAudit(t, repository.audits, "删除结果未知")
}

func TestManagementFailureReportsKeyOnlyPartialCompletion(t *testing.T) {
	repository := &deleteRepository{account: boundAccount()}
	keys := &deleteKeys{keys: []business.UpstreamCatalogKey{{KeyID: "key-8"}}}
	admin := &deleteAdmin{err: &adminclient.AccountReadbackUnknownError{
		AccountID: "37", ReadbackErr: errors.New("management unavailable"),
	}}
	private := configuredDeletePrivate()
	service := configuredServiceWithPrivate(repository, keys, admin, private)
	preview, _ := service.Preview(context.Background(), "37")
	result, err := service.Delete(context.Background(), preview, "tester")
	if err == nil || !strings.Contains(err.Error(), "账号删除结果未知") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.UpstreamKeyDeleted || !result.UpstreamKeyProjectionDeleted || !result.UpstreamKeySecretDeleted ||
		!result.ManagementAccountDeleteRequested || !result.ManagementAccountReadbackFailed ||
		result.ManagementAccountDeleted || result.LocalProjectionDeleted || repository.deleted || !repository.keyReconciled ||
		len(private.deletedSecrets) != 1 {
		t.Fatalf("partial result is incorrect: %#v", result)
	}
	assertPartialDeleteAudit(t, repository.audits, "management-readback", true, "账号删除结果未知")
}

func TestManagementStillReadableHasDistinctAuditAndKeepsConfirmedKeyReconciliation(t *testing.T) {
	repository := &deleteRepository{account: boundAccount()}
	keys := &deleteKeys{keys: []business.UpstreamCatalogKey{{KeyID: "key-8"}}}
	admin := &deleteAdmin{err: &adminclient.AccountStillReadableError{AccountID: "37"}}
	private := configuredDeletePrivate()
	service := configuredServiceWithPrivate(repository, keys, admin, private)
	preview, _ := service.Preview(context.Background(), "37")

	result, err := service.Delete(context.Background(), preview, "tester")
	if err == nil || !strings.Contains(err.Error(), "仍可读") || strings.Contains(err.Error(), "结果未知") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.UpstreamKeyDeleted || !result.UpstreamKeyProjectionDeleted || !result.UpstreamKeySecretDeleted ||
		!result.ManagementAccountDeleteRequested || !result.ManagementAccountStillReadable ||
		result.ManagementAccountReadbackFailed || result.ManagementAccountDeleted || repository.deleted ||
		!repository.keyReconciled || len(private.deletedSecrets) != 1 {
		t.Fatalf("still-readable result lost stage facts: %#v", result)
	}
	assertPartialDeleteAudit(t, repository.audits, "management-readback-still-readable", true, "仍可读")
}

func TestManagementOnlyDeleteErrorIsAuditedAsUnconfirmedRemoteWrite(t *testing.T) {
	account := boundAccount()
	account.Bindings = nil
	repository := &deleteRepository{account: account}
	keys := &deleteKeys{}
	admin := &deleteAdmin{err: &adminclient.AccountReadbackUnknownError{
		AccountID: "37", ReadbackErr: errors.New("management unavailable"),
	}}
	service := configuredService(repository, keys, admin)
	preview, _ := service.Preview(context.Background(), "37")

	result, err := service.Delete(context.Background(), preview, "tester")
	if err == nil || !strings.Contains(err.Error(), "删除结果未知") {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.UpstreamKeyDeleteRequested || result.UpstreamKeyDeleted || result.ReadbackConfirmed ||
		!result.ManagementAccountDeleteRequested || !result.ManagementAccountReadbackFailed ||
		result.ManagementAccountDeleted || result.LocalProjectionDeleted || keys.listCalls != 0 ||
		len(keys.deleteCalls) != 0 || len(admin.calls) != 1 || repository.deleted {
		t.Fatalf("management write failure was overstated or continued: result=%#v keys=%#v admin=%#v local=%v",
			result, keys.deleteCalls, admin.calls, repository.deleted)
	}
	assertUnconfirmedManagementDeleteAudit(t, repository.audits, "删除结果未知")
}

func TestConfirmedKeyReconciliationFailureStopsSecretAndManagementDeletion(t *testing.T) {
	repository := &deleteRepository{account: boundAccount(), reconcileErr: errors.New("scope changed")}
	keys := &deleteKeys{keys: []business.UpstreamCatalogKey{{KeyID: "key-8"}}}
	admin := &deleteAdmin{}
	private := configuredDeletePrivate()
	service := configuredServiceWithPrivate(repository, keys, admin, private)
	preview, _ := service.Preview(context.Background(), "37")

	result, err := service.Delete(context.Background(), preview, "tester")
	if err == nil || !strings.Contains(err.Error(), "Key 本地投影对账失败") || !strings.Contains(err.Error(), "scope changed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.UpstreamKeyDeleted || result.UpstreamKeyProjectionDeleted || result.UpstreamKeySecretDeleted ||
		!repository.keyReconciled || len(private.deletedSecrets) != 0 || len(admin.calls) != 0 || repository.deleted {
		t.Fatalf("reconciliation failure continued deletion: result=%#v private=%#v admin=%#v",
			result, private.deletedSecrets, admin.calls)
	}
	assertPartialDeleteAudit(t, repository.audits, "upstream-key-reconcile", true, "本地投影对账失败")
}

func TestConfirmedKeySecretCleanupFailureStopsManagementDeletion(t *testing.T) {
	repository := &deleteRepository{account: boundAccount()}
	keys := &deleteKeys{keys: []business.UpstreamCatalogKey{{KeyID: "key-8"}}}
	admin := &deleteAdmin{}
	private := configuredDeletePrivate()
	private.secretDeleteErr = errors.New("private store unavailable")
	service := configuredServiceWithPrivate(repository, keys, admin, private)
	preview, _ := service.Preview(context.Background(), "37")

	result, err := service.Delete(context.Background(), preview, "tester")
	if err == nil || !strings.Contains(err.Error(), "Key 私有密钥清理失败") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.UpstreamKeyDeleted || !result.UpstreamKeyProjectionDeleted || result.UpstreamKeySecretDeleted ||
		!repository.keyReconciled || len(private.deletedSecrets) != 1 || len(admin.calls) != 0 || repository.deleted {
		t.Fatalf("secret cleanup failure continued deletion: result=%#v private=%#v admin=%#v",
			result, private.deletedSecrets, admin.calls)
	}
	assertPartialDeleteAudit(t, repository.audits, "upstream-key-secret-reconcile", true, "私有密钥清理失败")
}

func TestDeleteRejectsSharedStableUpstreamKeyBeforeRemoteMutation(t *testing.T) {
	repository := &deleteRepository{
		account:    boundAccount(),
		confirmErr: errors.New("上游 Key 仍被其他账号绑定或开户待续引用，拒绝删除共享 Key"),
	}
	keys := &deleteKeys{keys: []business.UpstreamCatalogKey{{KeyID: "key-8"}}}
	admin := &deleteAdmin{}
	service := configuredService(repository, keys, admin)
	preview, err := service.Preview(context.Background(), "37")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Delete(context.Background(), preview, "tester")
	if err == nil || !strings.Contains(err.Error(), "共享 Key") {
		t.Fatalf("unexpected shared-key error: %v", err)
	}
	if len(keys.deleteCalls) != 0 || len(admin.calls) != 0 || repository.deleted || len(repository.audits) != 0 {
		t.Fatalf("shared key reached a mutation: keys=%#v admin=%#v local=%v audits=%#v",
			keys.deleteCalls, admin.calls, repository.deleted, repository.audits)
	}
}

func TestDeleteRejectsBindingChangedAfterPreview(t *testing.T) {
	repository := &deleteRepository{account: boundAccount()}
	keys := &deleteKeys{keys: []business.UpstreamCatalogKey{{KeyID: "key-8"}}}
	admin := &deleteAdmin{}
	service := configuredService(repository, keys, admin)
	preview, _ := service.Preview(context.Background(), "37")
	repository.account.Bindings[0].UpstreamKeyID = "key-9"
	_, err := service.Delete(context.Background(), preview, "tester")
	if err == nil || !strings.Contains(err.Error(), "绑定已变化") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys.deleteCalls) != 0 || len(admin.calls) != 0 || repository.deleted {
		t.Fatal("remote or local deletion ran after binding changed")
	}
}

func TestPreviewAllowsNoBindingButRejectsAmbiguousOrIncompleteBindings(t *testing.T) {
	repository := &deleteRepository{account: boundAccount()}
	service := configuredService(repository, &deleteKeys{}, &deleteAdmin{})
	repository.account.Bindings = nil
	preview, err := service.Preview(context.Background(), "37")
	if err != nil || preview.Binding != nil {
		t.Fatalf("unbound preview=%#v error=%v", preview, err)
	}
	repository.account.Bindings = append(boundAccount().Bindings, boundAccount().Bindings[0])
	if _, err := service.Preview(context.Background(), "37"); err == nil || !strings.Contains(err.Error(), "2 个上游 Key 绑定") {
		t.Fatalf("multiple binding error=%v", err)
	}
	repository.account.Bindings = boundAccount().Bindings
	repository.account.Bindings[0].UpstreamID = ""
	if _, err := service.Preview(context.Background(), "37"); err == nil || !strings.Contains(err.Error(), "上游身份 ID") {
		t.Fatalf("missing stable upstream identity error=%v", err)
	}
}

func TestPreviewRejectsMutationProtectedAccount(t *testing.T) {
	repository := &deleteRepository{account: boundAccount(), protection: business.AccountMutationProtection{ManualPriority: true}}
	service := configuredService(repository, &deleteKeys{}, &deleteAdmin{})
	_, err := service.Preview(context.Background(), "37")
	if err == nil || !strings.Contains(err.Error(), "人工优先位") || !strings.Contains(err.Error(), "先解除人工管控") {
		t.Fatalf("unexpected protection error: %v", err)
	}
	if repository.accountReads != 0 {
		t.Fatal("protected account scope was read before rejecting the operation")
	}
}

func TestEnqueueRejectsHostChangedAfterClientPreview(t *testing.T) {
	repository := &deleteRepository{account: boundAccount()}
	service := configuredService(repository, &deleteKeys{}, &deleteAdmin{})
	expected, err := service.Preview(context.Background(), "37")
	if err != nil {
		t.Fatal(err)
	}
	expected.Binding.UpstreamHost = "https://old-upstream.example.com"
	_, err = service.Enqueue(context.Background(), "37", expected.Binding, expected.ManagementBaseURL, "tester")
	if err == nil || !strings.Contains(err.Error(), "账号绑定已变化") {
		t.Fatalf("unexpected scope-change error: %v", err)
	}
}

func TestEnqueueRejectsStableUpstreamIdentityChangedAfterClientPreview(t *testing.T) {
	repository := &deleteRepository{account: boundAccount()}
	service := configuredService(repository, &deleteKeys{}, &deleteAdmin{})
	expected, err := service.Preview(context.Background(), "37")
	if err != nil {
		t.Fatal(err)
	}
	repository.account.Bindings[0].UpstreamID = "upstream-2"
	_, err = service.Enqueue(context.Background(), "37", expected.Binding, expected.ManagementBaseURL, "tester")
	if err == nil || !strings.Contains(err.Error(), "账号绑定已变化") {
		t.Fatalf("unexpected identity-change error: %v", err)
	}
}

func TestEnqueueAcceptsStableScopeWithoutDisplayOnlyKeyName(t *testing.T) {
	repository := &deleteRepository{account: boundAccount()}
	tasks := &deleteTaskStore{}
	runner := &heldDeleteRunner{}
	service := New(repository, configuredDeletePrivate(), &deleteKeys{}, tasks)
	service.UseTaskRunner(runner)
	preview, err := service.Preview(context.Background(), "37")
	if err != nil {
		t.Fatal(err)
	}
	expected := *preview.Binding
	expected.UpstreamKeyName = ""

	if _, err := service.Enqueue(
		context.Background(), "37", &expected, preview.ManagementBaseURL, "tester",
	); err != nil {
		t.Fatalf("stable API scope without display-only Key name was rejected: %v", err)
	}
	if runner.run == nil || len(tasks.tasks) != 1 || tasks.tasks[0].Status != "queued" {
		t.Fatalf("delete task was not queued: runner=%v tasks=%#v", runner.run != nil, tasks.tasks)
	}
}

func TestQueuedDeleteRejectsManagementTargetChangedBeforeWorker(t *testing.T) {
	repository := &deleteRepository{account: boundAccount()}
	keys := &deleteKeys{keys: []business.UpstreamCatalogKey{{KeyID: "key-8"}}}
	private := configuredDeletePrivate()
	tasks := &deleteTaskStore{}
	runner := &heldDeleteRunner{}
	service := New(repository, private, keys, tasks)
	service.UseTaskRunner(runner)
	adminA, adminB := &deleteAdmin{}, &deleteAdmin{}
	service.adminFactory = func(target configstore.TargetSettings) (Admin, error) {
		if target.BaseURL == "https://management-b.example.com" {
			return adminB, nil
		}
		return adminA, nil
	}
	preview, err := service.Preview(context.Background(), "37")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Enqueue(
		context.Background(), "37", preview.Binding, preview.ManagementBaseURL, "tester",
	); err != nil {
		t.Fatal(err)
	}
	private.target.BaseURL = "https://management-b.example.com"
	if runner.run == nil {
		t.Fatal("delete worker was not queued")
	}
	runner.run(context.Background())
	if len(keys.deleteCalls) != 0 || len(adminA.calls) != 0 || len(adminB.calls) != 0 || repository.deleted {
		t.Fatalf("target B switch reached deletion: keys=%#v adminA=%#v adminB=%#v local=%v",
			keys.deleteCalls, adminA.calls, adminB.calls, repository.deleted)
	}
	if len(tasks.tasks) < 3 || tasks.tasks[len(tasks.tasks)-1].Status != "failed" ||
		!strings.Contains(tasks.tasks[len(tasks.tasks)-1].Message, "管理目标已变化") {
		t.Fatalf("target change was not persisted as a failed task: %#v", tasks.tasks)
	}
}

func TestDeletePinsTargetBeforeKeyAndRechecksBeforeAccountDelete(t *testing.T) {
	repository := &deleteRepository{account: boundAccount()}
	private := configuredDeletePrivate()
	keys := &deleteKeys{keys: []business.UpstreamCatalogKey{{KeyID: "key-8"}}}
	keys.afterDelete = func() { private.target.BaseURL = "https://management-b.example.com" }
	adminA, adminB := &deleteAdmin{}, &deleteAdmin{}
	service := configuredServiceWithPrivate(repository, keys, adminA, private)
	factoryTargets := []string{}
	service.adminFactory = func(target configstore.TargetSettings) (Admin, error) {
		factoryTargets = append(factoryTargets, target.BaseURL)
		if target.BaseURL == "https://management-b.example.com" {
			return adminB, nil
		}
		return adminA, nil
	}
	preview, err := service.Preview(context.Background(), "37")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Delete(context.Background(), preview, "tester")
	if err == nil || !strings.Contains(err.Error(), "管理目标已变化") {
		t.Fatalf("unexpected target-change error: %v", err)
	}
	if !result.UpstreamKeyDeleted || len(factoryTargets) != 1 || factoryTargets[0] != "https://management-a.example.com" ||
		!result.UpstreamKeyProjectionDeleted || !result.UpstreamKeySecretDeleted || !repository.keyReconciled ||
		len(private.deletedSecrets) != 1 || len(adminA.calls) != 0 || len(adminB.calls) != 0 || repository.deleted {
		t.Fatalf("management target was not pinned safely: result=%#v targets=%#v adminA=%#v adminB=%#v local=%v",
			result, factoryTargets, adminA.calls, adminB.calls, repository.deleted)
	}
	assertPartialDeleteAudit(t, repository.audits, "management-target-check", true, "管理目标已变化")
}

func TestAlreadyAbsentKeyThenTargetChangeAuditsNoRemoteWrite(t *testing.T) {
	repository := &deleteRepository{account: boundAccount()}
	private := configuredDeletePrivate()
	private.afterSecretDelete = func() { private.target.BaseURL = "https://management-b.example.com" }
	keys := &deleteKeys{}
	admin := &deleteAdmin{}
	service := configuredServiceWithPrivate(repository, keys, admin, private)
	preview, err := service.Preview(context.Background(), "37")
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Delete(context.Background(), preview, "tester")
	if err == nil || !strings.Contains(err.Error(), "管理目标已变化") {
		t.Fatalf("unexpected target-change error: %v", err)
	}
	if result.UpstreamKeyDeleteRequested || !result.UpstreamKeyDeleted || !result.UpstreamKeyAlreadyAbsent ||
		!result.UpstreamKeyProjectionDeleted || !result.UpstreamKeySecretDeleted || len(admin.calls) != 0 ||
		repository.deleted || !repository.keyReconciled {
		t.Fatalf("already-absent key result was inaccurate: result=%#v admin=%#v", result, admin.calls)
	}
	assertPartialDeleteAudit(t, repository.audits, "management-target-check", false, "管理目标已变化")
}

func assertPartialDeleteAudit(t *testing.T, audits []business.AccountOperation, phase string, writeback bool, message string) {
	t.Helper()
	if len(audits) != 1 {
		t.Fatalf("partial remote result was not audited exactly once: %#v", audits)
	}
	audit := audits[0]
	if audit.OperationType != "account.delete" || audit.State != "failed" || audit.Phase != phase ||
		audit.ObjectID != "37" || audit.Error == nil || !strings.Contains(*audit.Error, message) ||
		audit.RemoteConfirmed || audit.ReadbackConfirmed || audit.Writeback != writeback {
		t.Fatalf("partial audit metadata is incomplete: %#v", audit)
	}
	after, ok := audit.After.(map[string]any)
	if !ok || after["upstream_key_deleted"] != true || after["management_account_deleted"] != false ||
		after["local_projection_deleted"] != false ||
		after["upstream_key_projection_deleted"] != true && phase != "upstream-key-reconcile" {
		t.Fatalf("partial audit lied about deletion state: %#v", audit.After)
	}
}

func assertUnconfirmedKeyDeleteAudit(t *testing.T, audits []business.AccountOperation, message string) {
	t.Helper()
	if len(audits) != 1 {
		t.Fatalf("unconfirmed upstream write was not audited exactly once: %#v", audits)
	}
	audit := audits[0]
	if audit.OperationType != "account.delete" || audit.State != "failed" || audit.Phase != "upstream-key-readback" ||
		audit.ObjectID != "37" || audit.Error == nil || !strings.Contains(*audit.Error, message) ||
		audit.RemoteConfirmed || audit.ReadbackConfirmed || !audit.Writeback {
		t.Fatalf("unconfirmed upstream write audit is inaccurate: %#v", audit)
	}
	after, ok := audit.After.(map[string]any)
	if !ok || after["upstream_key_delete_requested"] != true || after["upstream_key_deleted"] != false ||
		after["upstream_key_already_absent"] != false || after["management_account_deleted"] != false ||
		after["local_projection_deleted"] != false {
		t.Fatalf("unconfirmed upstream write audit lied about deletion state: %#v", audit.After)
	}
}

func assertUnconfirmedManagementDeleteAudit(t *testing.T, audits []business.AccountOperation, message string) {
	t.Helper()
	if len(audits) != 1 {
		t.Fatalf("unconfirmed management write was not audited exactly once: %#v", audits)
	}
	audit := audits[0]
	if audit.OperationType != "account.delete" || audit.State != "failed" || audit.Phase != "management-readback" ||
		audit.ObjectID != "37" || audit.Error == nil || !strings.Contains(*audit.Error, message) ||
		audit.RemoteConfirmed || audit.ReadbackConfirmed || !audit.Writeback {
		t.Fatalf("unconfirmed management write audit is inaccurate: %#v", audit)
	}
	after, ok := audit.After.(map[string]any)
	if !ok || after["upstream_key_delete_requested"] != false || after["upstream_key_deleted"] != false ||
		after["upstream_key_already_absent"] != false || after["management_account_deleted"] != false ||
		after["local_projection_deleted"] != false {
		t.Fatalf("unconfirmed management write audit lied about deletion state: %#v", audit.After)
	}
}
