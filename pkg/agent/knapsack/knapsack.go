package knapsack

import (
	"time"

	"github.com/kolide/launcher/pkg/agent/flags/keys"
	"github.com/kolide/launcher/pkg/agent/storage"
	"github.com/kolide/launcher/pkg/agent/types"
	"go.etcd.io/bbolt"
)

// Knapsack is an inventory of data and useful services which are used throughout
// launcher code and are typically valid for the lifetime of the launcher application instance.
type knapsack struct {
	stores map[storage.Store]types.KVStore
	flags  types.Flags

	// BboltDB is the underlying bbolt database.
	// Ideally, we can eventually remove this. This is only here because some parts of the codebase
	// like the osquery extension have a direct dependency on bbolt and need this reference.
	// If we are able to abstract bbolt out completely in these areas, we should be able to
	// remove this field and prevent "leaking" bbolt into places it doesn't need to.
	db *bbolt.DB

	// This struct is a work in progress, and will be iteratively added to as needs arise.
	// Some potential future additions include:
	// Querier
}

func New(stores map[storage.Store]types.KVStore, flags types.Flags, db *bbolt.DB) *knapsack {
	k := &knapsack{
		db:     db,
		flags:  flags,
		stores: stores,
	}

	return k
}

// BboltDB interface methods

func (k *knapsack) BboltDB() *bbolt.DB {
	return k.db
}

// Stores interface methods

func (k *knapsack) AgentFlagsStore() types.KVStore {
	return k.getKVStore(storage.AgentFlagsStore)
}

func (k *knapsack) AutoupdateErrorsStore() types.KVStore {
	return k.getKVStore(storage.AutoupdateErrorsStore)
}

func (k *knapsack) ConfigStore() types.KVStore {
	return k.getKVStore(storage.ConfigStore)
}

func (k *knapsack) ControlStore() types.KVStore {
	return k.getKVStore(storage.ControlStore)
}

func (k *knapsack) InitialResultsStore() types.KVStore {
	return k.getKVStore(storage.InitialResultsStore)
}

func (k *knapsack) ResultLogsStore() types.KVStore {
	return k.getKVStore(storage.ResultLogsStore)
}

func (k *knapsack) OsqueryHistoryInstanceStore() types.KVStore {
	return k.getKVStore(storage.OsqueryHistoryInstanceStore)
}

func (k *knapsack) SentNotificationsStore() types.KVStore {
	return k.getKVStore(storage.SentNotificationsStore)
}

func (k *knapsack) StatusLogsStore() types.KVStore {
	return k.getKVStore(storage.StatusLogsStore)
}

func (k *knapsack) ServerProvidedDataStore() types.KVStore {
	return k.getKVStore(storage.ServerProvidedDataStore)
}

func (k *knapsack) getKVStore(storeType storage.Store) types.KVStore {
	if k == nil {
		return nil
	}

	// Ignoring ok value, this should only fail if an invalid storeType is provided
	store, _ := k.stores[storeType]
	return store
}

// Flags interface methods

func (k *knapsack) RegisterChangeObserver(observer types.FlagsChangeObserver, flagKeys ...keys.FlagKey) {
	k.flags.RegisterChangeObserver(observer, flagKeys...)
}

func (k *knapsack) AutoloadedExtensions() []string {
	return k.flags.AutoloadedExtensions()
}

func (k *knapsack) SetKolideServerURL(url string) error {
	return k.flags.SetKolideServerURL(url)
}
func (k *knapsack) KolideServerURL() string {
	return k.flags.KolideServerURL()
}

func (k *knapsack) SetKolideHosted(hosted bool) error {
	return k.flags.SetKolideHosted(hosted)
}
func (k *knapsack) KolideHosted() bool {
	return k.flags.KolideHosted()
}

func (k *knapsack) EnrollSecret() string {
	return k.flags.EnrollSecret()
}

func (k *knapsack) EnrollSecretPath() string {
	return k.flags.EnrollSecretPath()
}

func (k *knapsack) RootDirectory() string {
	return k.flags.RootDirectory()
}

func (k *knapsack) OsquerydPath() string {
	return k.flags.OsquerydPath()
}

func (k *knapsack) CertPins() [][]byte {
	return k.flags.CertPins()
}

func (k *knapsack) RootPEM() string {
	return k.flags.RootPEM()
}

func (k *knapsack) LoggingInterval() time.Duration {
	return k.flags.LoggingInterval()
}

func (k *knapsack) EnableInitialRunner() bool {
	return k.flags.EnableInitialRunner()
}

func (k *knapsack) Transport() string {
	return k.flags.Transport()
}

func (k *knapsack) LogMaxBytesPerBatch() int {
	return k.flags.LogMaxBytesPerBatch()
}

func (k *knapsack) SetDesktopEnabled(enabled bool) error {
	return k.flags.SetDesktopEnabled(enabled)
}
func (k *knapsack) DesktopEnabled() bool {
	return k.flags.DesktopEnabled()
}

func (k *knapsack) SetDesktopUpdateInterval(interval time.Duration) error {
	return k.flags.SetDesktopUpdateInterval(interval)
}
func (k *knapsack) DesktopUpdateInterval() time.Duration {
	return k.flags.DesktopUpdateInterval()
}

func (k *knapsack) SetDesktopMenuRefreshInterval(interval time.Duration) error {
	return k.flags.SetDesktopMenuRefreshInterval(interval)
}
func (k *knapsack) DesktopMenuRefreshInterval() time.Duration {
	return k.flags.DesktopMenuRefreshInterval()
}

func (k *knapsack) SetDebugServerData(debug bool) error {
	return k.flags.SetDebugServerData(debug)
}
func (k *knapsack) DebugServerData() bool {
	return k.flags.DebugServerData()
}

func (k *knapsack) SetForceControlSubsystems(force bool) error {
	return k.flags.SetForceControlSubsystems(force)
}
func (k *knapsack) ForceControlSubsystems() bool {
	return k.flags.ForceControlSubsystems()
}

func (k *knapsack) SetControlServerURL(url string) error {
	return k.flags.SetControlServerURL(url)
}
func (k *knapsack) ControlServerURL() string {
	return k.flags.ControlServerURL()
}

func (k *knapsack) SetControlRequestInterval(interval time.Duration) error {
	return k.flags.SetControlRequestInterval(interval)
}
func (k *knapsack) SetControlRequestIntervalOverride(interval, duration time.Duration) {
	k.flags.SetControlRequestIntervalOverride(interval, duration)
}
func (k *knapsack) ControlRequestInterval() time.Duration {
	return k.flags.ControlRequestInterval()
}

func (k *knapsack) SetDisableControlTLS(disabled bool) error {
	return k.flags.SetDisableControlTLS(disabled)
}
func (k *knapsack) DisableControlTLS() bool {
	return k.flags.DisableControlTLS()
}

func (k *knapsack) SetInsecureControlTLS(disabled bool) error {
	return k.flags.SetInsecureControlTLS(disabled)
}
func (k *knapsack) InsecureControlTLS() bool {
	return k.flags.InsecureControlTLS()
}

func (k *knapsack) SetInsecureTLS(insecure bool) error {
	return k.flags.SetInsecureTLS(insecure)
}
func (k *knapsack) InsecureTLS() bool {
	return k.flags.InsecureTLS()
}

func (k *knapsack) SetInsecureTransportTLS(insecure bool) error {
	return k.flags.SetInsecureTransportTLS(insecure)
}
func (k *knapsack) InsecureTransportTLS() bool {
	return k.flags.InsecureTransportTLS()
}

func (k *knapsack) SetIAmBreakingEELicense(disabled bool) error {
	return k.flags.SetIAmBreakingEELicense(disabled)
}
func (k *knapsack) IAmBreakingEELicense() bool {
	return k.flags.IAmBreakingEELicense()
}

func (k *knapsack) SetDebug(debug bool) error {
	return k.flags.SetDebug(debug)
}
func (k *knapsack) Debug() bool {
	return k.flags.Debug()
}

func (k *knapsack) SetDebugLogFile(file string) error {
	return k.flags.SetDebugLogFile(file)
}
func (k *knapsack) DebugLogFile() string {
	return k.flags.DebugLogFile()
}

func (k *knapsack) SetOsqueryVerbose(verbose bool) error {
	return k.flags.SetOsqueryVerbose(verbose)
}
func (k *knapsack) OsqueryVerbose() bool {
	return k.flags.OsqueryVerbose()
}

func (k *knapsack) OsqueryFlags() []string {
	return k.flags.OsqueryFlags()
}

func (k *knapsack) OsqueryTlsConfigEndpoint() string {
	return k.flags.OsqueryTlsConfigEndpoint()
}
func (k *knapsack) OsqueryTlsEnrollEndpoint() string {
	return k.flags.OsqueryTlsEnrollEndpoint()
}
func (k *knapsack) OsqueryTlsLoggerEndpoint() string {
	return k.flags.OsqueryTlsLoggerEndpoint()
}
func (k *knapsack) OsqueryTlsDistributedReadEndpoint() string {
	return k.flags.OsqueryTlsDistributedReadEndpoint()
}
func (k *knapsack) OsqueryTlsDistributedWriteEndpoint() string {
	return k.flags.OsqueryTlsDistributedWriteEndpoint()
}

func (k *knapsack) SetAutoupdate(enabled bool) error {
	return k.flags.SetAutoupdate(enabled)
}
func (k *knapsack) Autoupdate() bool {
	return k.flags.Autoupdate()
}

func (k *knapsack) SetNotaryServerURL(url string) error {
	return k.flags.SetNotaryServerURL(url)
}
func (k *knapsack) NotaryServerURL() string {
	return k.flags.NotaryServerURL()
}

func (k *knapsack) SetTufServerURL(url string) error {
	return k.flags.SetTufServerURL(url)
}
func (k *knapsack) TufServerURL() string {
	return k.flags.TufServerURL()
}

func (k *knapsack) SetMirrorServerURL(url string) error {
	return k.flags.SetMirrorServerURL(url)
}
func (k *knapsack) MirrorServerURL() string {
	return k.flags.MirrorServerURL()
}

func (k *knapsack) SetAutoupdateInterval(interval time.Duration) error {
	return k.flags.SetAutoupdateInterval(interval)
}
func (k *knapsack) AutoupdateInterval() time.Duration {
	return k.flags.AutoupdateInterval()
}

func (k *knapsack) SetUpdateChannel(channel string) error {
	return k.flags.SetUpdateChannel(channel)
}
func (k *knapsack) UpdateChannel() string {
	return k.flags.UpdateChannel()
}

func (k *knapsack) SetNotaryPrefix(prefix string) error {
	return k.flags.SetNotaryPrefix(prefix)
}
func (k *knapsack) NotaryPrefix() string {
	return k.flags.NotaryPrefix()
}

func (k *knapsack) SetAutoupdateInitialDelay(delay time.Duration) error {
	return k.flags.SetAutoupdateInitialDelay(delay)
}
func (k *knapsack) AutoupdateInitialDelay() time.Duration {
	return k.flags.AutoupdateInitialDelay()
}

func (k *knapsack) SetUpdateDirectory(directory string) error {
	return k.flags.SetUpdateDirectory(directory)
}
func (k *knapsack) UpdateDirectory() string {
	return k.flags.UpdateDirectory()
}
