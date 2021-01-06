package wlan

const getbssid = `
Function Get-BSSID {
$NativeWifiCode = @'
using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Net.NetworkInformation;
using System.Threading;
using System.Text;
using System.Diagnostics;

namespace NativeWifi
{
    public static class Wlan
    {
        #region P/Invoke API
        public enum WlanIntfOpcode
        {
            AutoconfEnabled = 1,
            BackgroundScanEnabled,
            MediaStreamingMode,
            RadioState,
            BssType,
            InterfaceState,
            CurrentConnection,
            ChannelNumber,
            SupportedInfrastructureAuthCipherPairs,
            SupportedAdhocAuthCipherPairs,
            SupportedCountryOrRegionStringList,
            CurrentOperationMode,
            Statistics = 0x10000101,
            RSSI,
            SecurityStart = 0x20010000,
            SecurityEnd = 0x2fffffff,
            IhvStart = 0x30000000,
            IhvEnd = 0x3fffffff
        }

        public enum WlanOpcodeValueType
        {
            QueryOnly = 0,
            SetByGroupPolicy = 1,
            SetByUser = 2,
            Invalid = 3
        }

        public const uint WLAN_CLIENT_VERSION_XP_SP2 = 1;
        public const uint WLAN_CLIENT_VERSION_LONGHORN = 2;

        [DllImport("wlanapi.dll")]
        public static extern int WlanOpenHandle(
            [In] UInt32 clientVersion,
            [In, Out] IntPtr pReserved,
            [Out] out UInt32 negotiatedVersion,
            [Out] out IntPtr clientHandle);

        [DllImport("wlanapi.dll")]
        public static extern int WlanCloseHandle(
            [In] IntPtr clientHandle,
            [In, Out] IntPtr pReserved);

        [DllImport("wlanapi.dll")]
        public static extern int WlanEnumInterfaces(
            [In] IntPtr clientHandle,
            [In, Out] IntPtr pReserved,
            [Out] out IntPtr ppInterfaceList);

        [DllImport("wlanapi.dll")]
        public static extern int WlanQueryInterface(
            [In] IntPtr clientHandle,
            [In, MarshalAs(UnmanagedType.LPStruct)] Guid interfaceGuid,
            [In] WlanIntfOpcode opCode,
            [In, Out] IntPtr pReserved,
            [Out] out int dataSize,
            [Out] out IntPtr ppData,
            [Out] out WlanOpcodeValueType wlanOpcodeValueType);

        [DllImport("wlanapi.dll")]
        public static extern int WlanSetInterface(
            [In] IntPtr clientHandle,
            [In, MarshalAs(UnmanagedType.LPStruct)] Guid interfaceGuid,
            [In] WlanIntfOpcode opCode,
            [In] uint dataSize,
            [In] IntPtr pData,
            [In, Out] IntPtr pReserved);

        [DllImport("wlanapi.dll")]
        public static extern int WlanScan(
            [In] IntPtr clientHandle,
            [In, MarshalAs(UnmanagedType.LPStruct)] Guid interfaceGuid,
            [In] IntPtr pDot11Ssid,
            [In] IntPtr pIeData,
            [In, Out] IntPtr pReserved);

        [Flags]
        public enum WlanGetAvailableNetworkFlags
        {
            IncludeAllAdhocProfiles = 0x00000001,
            IncludeAllManualHiddenProfiles = 0x00000002
        }

        [StructLayout(LayoutKind.Sequential)]
        internal struct WlanAvailableNetworkListHeader
        {
            public uint numberOfItems;
            public uint index;
        }

        [Flags]
        public enum WlanAvailableNetworkFlags
        {
            Connected = 0x00000001,
            HasProfile = 0x00000002
        }

        [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
        public struct WlanAvailableNetwork
        {
            [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 256)]
            public string profileName;
            public Dot11Ssid dot11Ssid;
            public Dot11BssType dot11BssType;
            public uint numberOfBssids;
            public bool networkConnectable;
            public WlanReasonCode wlanNotConnectableReason;
            private uint numberOfPhyTypes;
            [MarshalAs(UnmanagedType.ByValArray, SizeConst = 8)]
            private Dot11PhyType[] dot11PhyTypes;
            public Dot11PhyType[] Dot11PhyTypes
            {
                get
                {
                    Dot11PhyType[] ret = new Dot11PhyType[numberOfPhyTypes];
                    Array.Copy(dot11PhyTypes, ret, numberOfPhyTypes);
                    return ret;
                }
            }
            public bool morePhyTypes;
            public uint wlanSignalQuality;
            public bool securityEnabled;
            public Dot11AuthAlgorithm dot11DefaultAuthAlgorithm;
            public Dot11CipherAlgorithm dot11DefaultCipherAlgorithm;
            public WlanAvailableNetworkFlags flags;
            uint reserved;
        }

        [DllImport("wlanapi.dll")]
        public static extern int WlanGetAvailableNetworkList(
            [In] IntPtr clientHandle,
            [In, MarshalAs(UnmanagedType.LPStruct)] Guid interfaceGuid,
            [In] WlanGetAvailableNetworkFlags flags,
            [In, Out] IntPtr reservedPtr,
            [Out] out IntPtr availableNetworkListPtr);

        [Flags]
        public enum WlanProfileFlags
        {
            AllUser = 0,
            GroupPolicy = 1,
            User = 2
        }

        [DllImport("wlanapi.dll")]
        public static extern int WlanSetProfile(
            [In] IntPtr clientHandle,
            [In, MarshalAs(UnmanagedType.LPStruct)] Guid interfaceGuid,
            [In] WlanProfileFlags flags,
            [In, MarshalAs(UnmanagedType.LPWStr)] string profileXml,
            [In, Optional, MarshalAs(UnmanagedType.LPWStr)] string allUserProfileSecurity,
            [In] bool overwrite,
            [In] IntPtr pReserved,
            [Out] out WlanReasonCode reasonCode);

        [Flags]
        public enum WlanAccess
        {
            ReadAccess = 0x00020000 | 0x0001,
            ExecuteAccess = ReadAccess | 0x0020,
            WriteAccess = ReadAccess | ExecuteAccess | 0x0002 | 0x00010000 | 0x00040000
        }

        [DllImport("wlanapi.dll")]
        public static extern int WlanGetProfile(
            [In] IntPtr clientHandle,
            [In, MarshalAs(UnmanagedType.LPStruct)] Guid interfaceGuid,
            [In, MarshalAs(UnmanagedType.LPWStr)] string profileName,
            [In] IntPtr pReserved,
            [Out] out IntPtr profileXml,
            [Out, Optional] out WlanProfileFlags flags,
            [Out, Optional] out WlanAccess grantedAccess);

        [DllImport("wlanapi.dll")]
        public static extern int WlanGetProfileList(
            [In] IntPtr clientHandle,
            [In, MarshalAs(UnmanagedType.LPStruct)] Guid interfaceGuid,
            [In] IntPtr pReserved,
            [Out] out IntPtr profileList
        );

        [DllImport("wlanapi.dll")]
        public static extern void WlanFreeMemory(IntPtr pMemory);

        [DllImport("wlanapi.dll")]
        public static extern int WlanReasonCodeToString(
            [In] WlanReasonCode reasonCode,
            [In] int bufferSize,
            [In, Out] StringBuilder stringBuffer,
            IntPtr pReserved
        );

        [Flags]
        public enum WlanNotificationSource
        {
            None = 0,
            All = 0X0000FFFF,
            ACM = 0X00000008,
            MSM = 0X00000010,
            Security = 0X00000020,
            IHV = 0X00000040
        }

        public enum WlanNotificationCodeAcm
        {
            AutoconfEnabled = 1,
            AutoconfDisabled,
            BackgroundScanEnabled,
            BackgroundScanDisabled,
            BssTypeChange,
            PowerSettingChange,
            ScanComplete,
            ScanFail,
            ConnectionStart,
            ConnectionComplete,
            ConnectionAttemptFail,
            FilterListChange,
            InterfaceArrival,
            InterfaceRemoval,
            ProfileChange,
            ProfileNameChange,
            ProfilesExhausted,
            NetworkNotAvailable,
            NetworkAvailable,
            Disconnecting,
            Disconnected,
            AdhocNetworkStateChange
        }

        public enum WlanNotificationCodeMsm
        {
            Associating = 1,
            Associated,
            Authenticating,
            Connected,
            RoamingStart,
            RoamingEnd,
            RadioStateChange,
            SignalQualityChange,
            Disassociating,
            Disconnected,
            PeerJoin,
            PeerLeave,
            AdapterRemoval,
            AdapterOperationModeChange
        }

        [StructLayout(LayoutKind.Sequential)]
        public struct WlanNotificationData
        {
            public WlanNotificationSource notificationSource;
            public int notificationCode;
            public Guid interfaceGuid;
            public int dataSize;
            public IntPtr dataPtr;

            public object NotificationCode
            {
                get
                {
                    if (notificationSource == WlanNotificationSource.MSM)
                        return (WlanNotificationCodeMsm)notificationCode;
                    else if (notificationSource == WlanNotificationSource.ACM)
                        return (WlanNotificationCodeAcm)notificationCode;
                    else
                        return notificationCode;
                }

            }
        }

        public delegate void WlanNotificationCallbackDelegate(ref WlanNotificationData notificationData, IntPtr context);

        [DllImport("wlanapi.dll")]
        public static extern int WlanRegisterNotification(
            [In] IntPtr clientHandle,
            [In] WlanNotificationSource notifSource,
            [In] bool ignoreDuplicate,
            [In] WlanNotificationCallbackDelegate funcCallback,
            [In] IntPtr callbackContext,
            [In] IntPtr reserved,
            [Out] out WlanNotificationSource prevNotifSource);

        [Flags]
        public enum WlanConnectionFlags
        {
            HiddenNetwork = 0x00000001,
            AdhocJoinOnly = 0x00000002,
            IgnorePrivacyBit = 0x00000004,
            EapolPassthrough = 0x00000008
        }

        [StructLayout(LayoutKind.Sequential)]
        public struct WlanConnectionParameters
        {
            public WlanConnectionMode wlanConnectionMode;
            [MarshalAs(UnmanagedType.LPWStr)]
            public string profile;
            public IntPtr dot11SsidPtr;
            public IntPtr desiredBssidListPtr;
            public Dot11BssType dot11BssType;
            public WlanConnectionFlags flags;
        }

        public enum WlanAdhocNetworkState
        {
            Formed = 0,
            Connected = 1
        }

        [DllImport("wlanapi.dll")]
        public static extern int WlanConnect(
            [In] IntPtr clientHandle,
            [In, MarshalAs(UnmanagedType.LPStruct)] Guid interfaceGuid,
            [In] ref WlanConnectionParameters connectionParameters,
            IntPtr pReserved);

        [DllImport("wlanapi.dll")]
        public static extern int WlanDeleteProfile(
            [In] IntPtr clientHandle,
            [In, MarshalAs(UnmanagedType.LPStruct)] Guid interfaceGuid,
            [In, MarshalAs(UnmanagedType.LPWStr)] string profileName,
            IntPtr reservedPtr
        );

        [DllImport("wlanapi.dll")]
        public static extern int WlanGetNetworkBssList(
            [In] IntPtr clientHandle,
            [In, MarshalAs(UnmanagedType.LPStruct)] Guid interfaceGuid,
            [In] IntPtr dot11SsidInt,
            [In] Dot11BssType dot11BssType,
            [In] bool securityEnabled,
            IntPtr reservedPtr,
            [Out] out IntPtr wlanBssList
        );

        [StructLayout(LayoutKind.Sequential)]
        internal struct WlanBssListHeader
        {
            internal uint totalSize;
            internal uint numberOfItems;
        }

        [StructLayout(LayoutKind.Sequential)]
        public struct WlanBssEntry
        {
            public Dot11Ssid dot11Ssid;
            public uint phyId;
            [MarshalAs(UnmanagedType.ByValArray, SizeConst = 6)]
            public byte[] dot11Bssid;
            public Dot11BssType dot11BssType;
            public Dot11PhyType dot11BssPhyType;
            public int rssi;
            public uint linkQuality;
            public bool inRegDomain;
            public ushort beaconPeriod;
            public ulong timestamp;
            public ulong hostTimestamp;
            public ushort capabilityInformation;
            public uint chCenterFrequency;
            public WlanRateSet wlanRateSet;
            public uint ieOffset;
            public uint ieSize;
        }

        [StructLayout(LayoutKind.Sequential)]
        public struct WlanRateSet
        {
            private uint rateSetLength;
            [MarshalAs(UnmanagedType.ByValArray, SizeConst = 126)]
            private ushort[] rateSet;

            public ushort[] Rates
            {
                get
                {
                    ushort[] rates = new ushort[rateSetLength / sizeof(ushort)];
                    Array.Copy(rateSet, rates, rates.Length);
                    return rates;
                }
            }

            public double GetRateInMbps(int rate)
            {
                return (rateSet[rate] & 0x7FFF) * 0.5;
            }
        }

        public class WlanException : Exception
        {
            private WlanReasonCode reasonCode;

            WlanException(WlanReasonCode reasonCode)
            {
                this.reasonCode = reasonCode;
            }

            public WlanReasonCode ReasonCode
            {
                get { return reasonCode; }
            }

            public override string Message
            {
                get
                {
                    StringBuilder sb = new StringBuilder(1024);
                    if (WlanReasonCodeToString(reasonCode, sb.Capacity, sb, IntPtr.Zero) == 0)
                        return sb.ToString();
                    else
                        return string.Empty;
                }
            }
        }

        // TODO: .NET-ify the WlanReasonCode enum (naming convention + docs).

        public enum WlanReasonCode
        {
            Success = 0,
            // general codes
            UNKNOWN = 0x10000 + 1,

            RANGE_SIZE = 0x10000,
            BASE = 0x10000 + RANGE_SIZE,

            // range for Auto Config
            //
            AC_BASE = 0x10000 + RANGE_SIZE,
            AC_CONNECT_BASE = (AC_BASE + RANGE_SIZE / 2),
            AC_END = (AC_BASE + RANGE_SIZE - 1),

            // range for profile manager
            // it has profile adding failure reason codes, but may not have
            // connection reason codes
            //
            PROFILE_BASE = 0x10000 + (7 * RANGE_SIZE),
            PROFILE_CONNECT_BASE = (PROFILE_BASE + RANGE_SIZE / 2),
            PROFILE_END = (PROFILE_BASE + RANGE_SIZE - 1),

            // range for MSM
            //
            MSM_BASE = 0x10000 + (2 * RANGE_SIZE),
            MSM_CONNECT_BASE = (MSM_BASE + RANGE_SIZE / 2),
            MSM_END = (MSM_BASE + RANGE_SIZE - 1),

            // range for MSMSEC
            //
            MSMSEC_BASE = 0x10000 + (3 * RANGE_SIZE),
            MSMSEC_CONNECT_BASE = (MSMSEC_BASE + RANGE_SIZE / 2),
            MSMSEC_END = (MSMSEC_BASE + RANGE_SIZE - 1),

            // AC network incompatible reason codes
            //
            NETWORK_NOT_COMPATIBLE = (AC_BASE + 1),
            PROFILE_NOT_COMPATIBLE = (AC_BASE + 2),

            // AC connect reason code
            //
            NO_AUTO_CONNECTION = (AC_CONNECT_BASE + 1),
            NOT_VISIBLE = (AC_CONNECT_BASE + 2),
            GP_DENIED = (AC_CONNECT_BASE + 3),
            USER_DENIED = (AC_CONNECT_BASE + 4),
            BSS_TYPE_NOT_ALLOWED = (AC_CONNECT_BASE + 5),
            IN_FAILED_LIST = (AC_CONNECT_BASE + 6),
            IN_BLOCKED_LIST = (AC_CONNECT_BASE + 7),
            SSID_LIST_TOO_LONG = (AC_CONNECT_BASE + 8),
            CONNECT_CALL_FAIL = (AC_CONNECT_BASE + 9),
            SCAN_CALL_FAIL = (AC_CONNECT_BASE + 10),
            NETWORK_NOT_AVAILABLE = (AC_CONNECT_BASE + 11),
            PROFILE_CHANGED_OR_DELETED = (AC_CONNECT_BASE + 12),
            KEY_MISMATCH = (AC_CONNECT_BASE + 13),
            USER_NOT_RESPOND = (AC_CONNECT_BASE + 14),

            // Profile validation errors
            //
            INVALID_PROFILE_SCHEMA = (PROFILE_BASE + 1),
            PROFILE_MISSING = (PROFILE_BASE + 2),
            INVALID_PROFILE_NAME = (PROFILE_BASE + 3),
            INVALID_PROFILE_TYPE = (PROFILE_BASE + 4),
            INVALID_PHY_TYPE = (PROFILE_BASE + 5),
            MSM_SECURITY_MISSING = (PROFILE_BASE + 6),
            IHV_SECURITY_NOT_SUPPORTED = (PROFILE_BASE + 7),
            IHV_OUI_MISMATCH = (PROFILE_BASE + 8),
            // IHV OUI not present but there is IHV settings in profile
            IHV_OUI_MISSING = (PROFILE_BASE + 9),
            // IHV OUI is present but there is no IHV settings in profile
            IHV_SETTINGS_MISSING = (PROFILE_BASE + 10),
            // both/conflict MSMSec and IHV security settings exist in profile
            CONFLICT_SECURITY = (PROFILE_BASE + 11),
            // no IHV or MSMSec security settings in profile
            SECURITY_MISSING = (PROFILE_BASE + 12),
            INVALID_BSS_TYPE = (PROFILE_BASE + 13),
            INVALID_ADHOC_CONNECTION_MODE = (PROFILE_BASE + 14),
            NON_BROADCAST_SET_FOR_ADHOC = (PROFILE_BASE + 15),
            AUTO_SWITCH_SET_FOR_ADHOC = (PROFILE_BASE + 16),
            AUTO_SWITCH_SET_FOR_MANUAL_CONNECTION = (PROFILE_BASE + 17),
            IHV_SECURITY_ONEX_MISSING = (PROFILE_BASE + 18),
            PROFILE_SSID_INVALID = (PROFILE_BASE + 19),
            TOO_MANY_SSID = (PROFILE_BASE + 20),

            // MSM network incompatible reasons
            //
            UNSUPPORTED_SECURITY_SET_BY_OS = (MSM_BASE + 1),
            UNSUPPORTED_SECURITY_SET = (MSM_BASE + 2),
            BSS_TYPE_UNMATCH = (MSM_BASE + 3),
            PHY_TYPE_UNMATCH = (MSM_BASE + 4),
            DATARATE_UNMATCH = (MSM_BASE + 5),

            // MSM connection failure reasons, to be defined
            // failure reason codes
            //
            // user called to disconnect
            USER_CANCELLED = (MSM_CONNECT_BASE + 1),
            // got disconnect while associating
            ASSOCIATION_FAILURE = (MSM_CONNECT_BASE + 2),
            // timeout for association
            ASSOCIATION_TIMEOUT = (MSM_CONNECT_BASE + 3),
            // pre-association security completed with failure
            PRE_SECURITY_FAILURE = (MSM_CONNECT_BASE + 4),
            // fail to start post-association security
            START_SECURITY_FAILURE = (MSM_CONNECT_BASE + 5),
            // post-association security completed with failure
            SECURITY_FAILURE = (MSM_CONNECT_BASE + 6),
            // security watchdog timeout
            SECURITY_TIMEOUT = (MSM_CONNECT_BASE + 7),
            // got disconnect from driver when roaming
            ROAMING_FAILURE = (MSM_CONNECT_BASE + 8),
            // failed to start security for roaming
            ROAMING_SECURITY_FAILURE = (MSM_CONNECT_BASE + 9),
            // failed to start security for adhoc-join
            ADHOC_SECURITY_FAILURE = (MSM_CONNECT_BASE + 10),
            // got disconnection from driver
            DRIVER_DISCONNECTED = (MSM_CONNECT_BASE + 11),
            // driver operation failed
            DRIVER_OPERATION_FAILURE = (MSM_CONNECT_BASE + 12),
            // Ihv service is not available
            IHV_NOT_AVAILABLE = (MSM_CONNECT_BASE + 13),
            // Response from ihv timed out
            IHV_NOT_RESPONDING = (MSM_CONNECT_BASE + 14),
            // Timed out waiting for driver to disconnect
            DISCONNECT_TIMEOUT = (MSM_CONNECT_BASE + 15),
            // An internal error prevented the operation from being completed.
            INTERNAL_FAILURE = (MSM_CONNECT_BASE + 16),
            // UI Request timed out.
            UI_REQUEST_TIMEOUT = (MSM_CONNECT_BASE + 17),
            // Roaming too often, post security is not completed after 5 times.
            TOO_MANY_SECURITY_ATTEMPTS = (MSM_CONNECT_BASE + 18),

            // MSMSEC reason codes
            //

            MSMSEC_MIN = MSMSEC_BASE,

            // Key index specified is not valid
            MSMSEC_PROFILE_INVALID_KEY_INDEX = (MSMSEC_BASE + 1),
            // Key required, PSK present
            MSMSEC_PROFILE_PSK_PRESENT = (MSMSEC_BASE + 2),
            // Invalid key length
            MSMSEC_PROFILE_KEY_LENGTH = (MSMSEC_BASE + 3),
            // Invalid PSK length
            MSMSEC_PROFILE_PSK_LENGTH = (MSMSEC_BASE + 4),
            // No auth/cipher specified
            MSMSEC_PROFILE_NO_AUTH_CIPHER_SPECIFIED = (MSMSEC_BASE + 5),
            // Too many auth/cipher specified
            MSMSEC_PROFILE_TOO_MANY_AUTH_CIPHER_SPECIFIED = (MSMSEC_BASE + 6),
            // Profile contains duplicate auth/cipher
            MSMSEC_PROFILE_DUPLICATE_AUTH_CIPHER = (MSMSEC_BASE + 7),
            // Profile raw data is invalid (1x or key data)
            MSMSEC_PROFILE_RAWDATA_INVALID = (MSMSEC_BASE + 8),
            // Invalid auth/cipher combination
            MSMSEC_PROFILE_INVALID_AUTH_CIPHER = (MSMSEC_BASE + 9),
            // 802.1x disabled when it's required to be enabled
            MSMSEC_PROFILE_ONEX_DISABLED = (MSMSEC_BASE + 10),
            // 802.1x enabled when it's required to be disabled
            MSMSEC_PROFILE_ONEX_ENABLED = (MSMSEC_BASE + 11),
            MSMSEC_PROFILE_INVALID_PMKCACHE_MODE = (MSMSEC_BASE + 12),
            MSMSEC_PROFILE_INVALID_PMKCACHE_SIZE = (MSMSEC_BASE + 13),
            MSMSEC_PROFILE_INVALID_PMKCACHE_TTL = (MSMSEC_BASE + 14),
            MSMSEC_PROFILE_INVALID_PREAUTH_MODE = (MSMSEC_BASE + 15),
            MSMSEC_PROFILE_INVALID_PREAUTH_THROTTLE = (MSMSEC_BASE + 16),
            // PreAuth enabled when PMK cache is disabled
            MSMSEC_PROFILE_PREAUTH_ONLY_ENABLED = (MSMSEC_BASE + 17),
            // Capability matching failed at network
            MSMSEC_CAPABILITY_NETWORK = (MSMSEC_BASE + 18),
            // Capability matching failed at NIC
            MSMSEC_CAPABILITY_NIC = (MSMSEC_BASE + 19),
            // Capability matching failed at profile
            MSMSEC_CAPABILITY_PROFILE = (MSMSEC_BASE + 20),
            // Network does not support specified discovery type
            MSMSEC_CAPABILITY_DISCOVERY = (MSMSEC_BASE + 21),
            // Passphrase contains invalid character
            MSMSEC_PROFILE_PASSPHRASE_CHAR = (MSMSEC_BASE + 22),
            // Key material contains invalid character
            MSMSEC_PROFILE_KEYMATERIAL_CHAR = (MSMSEC_BASE + 23),
            // Wrong key type specified for the auth/cipher pair
            MSMSEC_PROFILE_WRONG_KEYTYPE = (MSMSEC_BASE + 24),
            // "Mixed cell" suspected (AP not beaconing privacy, we have privacy enabled profile)
            MSMSEC_MIXED_CELL = (MSMSEC_BASE + 25),
            // Auth timers or number of timeouts in profile is incorrect
            MSMSEC_PROFILE_AUTH_TIMERS_INVALID = (MSMSEC_BASE + 26),
            // Group key update interval in profile is incorrect
            MSMSEC_PROFILE_INVALID_GKEY_INTV = (MSMSEC_BASE + 27),
            // "Transition network" suspected, trying legacy 802.11 security
            MSMSEC_TRANSITION_NETWORK = (MSMSEC_BASE + 28),
            // Key contains characters which do not map to ASCII
            MSMSEC_PROFILE_KEY_UNMAPPED_CHAR = (MSMSEC_BASE + 29),
            // Capability matching failed at profile (auth not found)
            MSMSEC_CAPABILITY_PROFILE_AUTH = (MSMSEC_BASE + 30),
            // Capability matching failed at profile (cipher not found)
            MSMSEC_CAPABILITY_PROFILE_CIPHER = (MSMSEC_BASE + 31),

            // Failed to queue UI request
            MSMSEC_UI_REQUEST_FAILURE = (MSMSEC_CONNECT_BASE + 1),
            // 802.1x authentication did not start within configured time
            MSMSEC_AUTH_START_TIMEOUT = (MSMSEC_CONNECT_BASE + 2),
            // 802.1x authentication did not complete within configured time
            MSMSEC_AUTH_SUCCESS_TIMEOUT = (MSMSEC_CONNECT_BASE + 3),
            // Dynamic key exchange did not start within configured time
            MSMSEC_KEY_START_TIMEOUT = (MSMSEC_CONNECT_BASE + 4),
            // Dynamic key exchange did not succeed within configured time
            MSMSEC_KEY_SUCCESS_TIMEOUT = (MSMSEC_CONNECT_BASE + 5),
            // Message 3 of 4 way handshake has no key data (RSN/WPA)
            MSMSEC_M3_MISSING_KEY_DATA = (MSMSEC_CONNECT_BASE + 6),
            // Message 3 of 4 way handshake has no IE (RSN/WPA)
            MSMSEC_M3_MISSING_IE = (MSMSEC_CONNECT_BASE + 7),
            // Message 3 of 4 way handshake has no Group Key (RSN)
            MSMSEC_M3_MISSING_GRP_KEY = (MSMSEC_CONNECT_BASE + 8),
            // Matching security capabilities of IE in M3 failed (RSN/WPA)
            MSMSEC_PR_IE_MATCHING = (MSMSEC_CONNECT_BASE + 9),
            // Matching security capabilities of Secondary IE in M3 failed (RSN)
            MSMSEC_SEC_IE_MATCHING = (MSMSEC_CONNECT_BASE + 10),
            // Required a pairwise key but AP configured only group keys
            MSMSEC_NO_PAIRWISE_KEY = (MSMSEC_CONNECT_BASE + 11),
            // Message 1 of group key handshake has no key data (RSN/WPA)
            MSMSEC_G1_MISSING_KEY_DATA = (MSMSEC_CONNECT_BASE + 12),
            // Message 1 of group key handshake has no group key
            MSMSEC_G1_MISSING_GRP_KEY = (MSMSEC_CONNECT_BASE + 13),
            // AP reset secure bit after connection was secured
            MSMSEC_PEER_INDICATED_INSECURE = (MSMSEC_CONNECT_BASE + 14),
            // 802.1x indicated there is no authenticator but profile requires 802.1x
            MSMSEC_NO_AUTHENTICATOR = (MSMSEC_CONNECT_BASE + 15),
            // Plumbing settings to NIC failed
            MSMSEC_NIC_FAILURE = (MSMSEC_CONNECT_BASE + 16),
            // Operation was cancelled by caller
            MSMSEC_CANCELLED = (MSMSEC_CONNECT_BASE + 17),
            // Key was in incorrect format
            MSMSEC_KEY_FORMAT = (MSMSEC_CONNECT_BASE + 18),
            // Security downgrade detected
            MSMSEC_DOWNGRADE_DETECTED = (MSMSEC_CONNECT_BASE + 19),
            // PSK mismatch suspected
            MSMSEC_PSK_MISMATCH_SUSPECTED = (MSMSEC_CONNECT_BASE + 20),
            // Forced failure because connection method was not secure
            MSMSEC_FORCED_FAILURE = (MSMSEC_CONNECT_BASE + 21),
            // ui request couldn't be queued or user pressed cancel
            MSMSEC_SECURITY_UI_FAILURE = (MSMSEC_CONNECT_BASE + 22),

            MSMSEC_MAX = MSMSEC_END
        }

        [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
        public struct WlanConnectionNotificationData
        {
            public WlanConnectionMode wlanConnectionMode;
            [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 32)]
            public string profileName;
            public Dot11Ssid dot11Ssid;
            public Dot11BssType dot11BssType;
            public bool securityEnabled;
            public WlanReasonCode wlanReasonCode;
            [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 1)]
            public string profileXml;
        }

        public enum WlanInterfaceState
        {
            NotReady = 0,
            Connected = 1,
            AdHocNetworkFormed = 2,
            Disconnecting = 3,
            Disconnected = 4,
            Associating = 5,
            Discovering = 6,
            Authenticating = 7
        }

        public struct Dot11Ssid
        {
            public uint SSIDLength;
            [MarshalAs(UnmanagedType.ByValArray, SizeConst = 32)]
            public byte[] SSID;
        }

        public enum Dot11PhyType : uint
        {
            Unknown = 0,
            Any = Unknown,
            FHSS = 1,
            DSSS = 2,
            IrBaseband = 3,
            OFDM = 4,
            HRDSSS = 5,
            ERP = 6,
            IHV_Start = 0x80000000,
            IHV_End = 0xffffffff
        }

        public enum Dot11BssType
        {
            Infrastructure = 1,
            Independent = 2,
            Any = 3
        }

        [StructLayout(LayoutKind.Sequential)]
        public struct WlanAssociationAttributes
        {
            public Dot11Ssid dot11Ssid;
            public Dot11BssType dot11BssType;
            [MarshalAs(UnmanagedType.ByValArray, SizeConst = 6)]
            public byte[] dot11Bssid;
            public Dot11PhyType dot11PhyType;
            public uint dot11PhyIndex;
            public uint wlanSignalQuality;
            public uint rxRate;
            public uint txRate;

            public PhysicalAddress Dot11Bssid
            {
                get { return new PhysicalAddress(dot11Bssid); }
            }
        }

        public enum WlanConnectionMode
        {
            Profile = 0,
            TemporaryProfile,
            DiscoverySecure,
            DiscoveryUnsecure,
            Auto,
            Invalid
        }

        public enum Dot11AuthAlgorithm : uint
        {
            IEEE80211_Open = 1,
            IEEE80211_SharedKey = 2,
            WPA = 3,
            WPA_PSK = 4,
            WPA_None = 5,
            RSNA = 6,
            RSNA_PSK = 7,
            IHV_Start = 0x80000000,
            IHV_End = 0xffffffff
        }

        public enum Dot11CipherAlgorithm : uint
        {
            None = 0x00,
            WEP40 = 0x01,
            TKIP = 0x02,
            CCMP = 0x04,
            WEP104 = 0x05,
            WPA_UseGroup = 0x100,
            RSN_UseGroup = 0x100,
            WEP = 0x101,
            IHV_Start = 0x80000000,
            IHV_End = 0xffffffff
        }

        [StructLayout(LayoutKind.Sequential)]
        public struct WlanSecurityAttributes
        {
            [MarshalAs(UnmanagedType.Bool)]
            public bool securityEnabled;
            [MarshalAs(UnmanagedType.Bool)]
            public bool oneXEnabled;
            public Dot11AuthAlgorithm dot11AuthAlgorithm;
            public Dot11CipherAlgorithm dot11CipherAlgorithm;
        }

        [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
        public struct WlanConnectionAttributes
        {
            public WlanInterfaceState isState;
            public WlanConnectionMode wlanConnectionMode;
            [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 256)]
            public string profileName;
            public WlanAssociationAttributes wlanAssociationAttributes;
            public WlanSecurityAttributes wlanSecurityAttributes;
        }

        [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
        public struct WlanInterfaceInfo
        {
            public Guid interfaceGuid;
            [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 256)]
            public string interfaceDescription;
            public WlanInterfaceState isState;
        }

        [StructLayout(LayoutKind.Sequential)]
        internal struct WlanInterfaceInfoListHeader
        {
            public uint numberOfItems;
            public uint index;
        }

        [StructLayout(LayoutKind.Sequential)]
        internal struct WlanProfileInfoListHeader
        {
            public uint numberOfItems;
            public uint index;
        }

        [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
        public struct WlanProfileInfo
        {
            [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 256)]
            public string profileName;
            public WlanProfileFlags profileFlags;
        }

        [Flags]
        public enum Dot11OperationMode : uint
        {
            Unknown = 0x00000000,
            Station = 0x00000001,
            AP = 0x00000002,
            ExtensibleStation = 0x00000004,
            NetworkMonitor = 0x80000000
        }
        #endregion

        [DebuggerStepThrough]
        internal static void ThrowIfError(int win32ErrorCode)
        {
            if (win32ErrorCode != 0)
                throw new Win32Exception(win32ErrorCode);
        }
    }
	public class WlanClient
	{
		public class WlanInterface
		{
			private WlanClient client;
			private Wlan.WlanInterfaceInfo info;

			#region Events
			public delegate void WlanNotificationEventHandler(Wlan.WlanNotificationData notifyData);

			public delegate void WlanConnectionNotificationEventHandler(Wlan.WlanNotificationData notifyData, Wlan.WlanConnectionNotificationData connNotifyData);

			public delegate void WlanReasonNotificationEventHandler(Wlan.WlanNotificationData notifyData, Wlan.WlanReasonCode reasonCode);

			public event WlanNotificationEventHandler WlanNotification;

			public event WlanConnectionNotificationEventHandler WlanConnectionNotification;

			public event WlanReasonNotificationEventHandler WlanReasonNotification;

			#endregion

			#region Event queue
			private bool queueEvents;
			private AutoResetEvent eventQueueFilled = new AutoResetEvent(false);
			private Queue<object> eventQueue = new Queue<object>();

			private struct WlanConnectionNotificationEventData
			{
				public Wlan.WlanNotificationData notifyData;
				public Wlan.WlanConnectionNotificationData connNotifyData;
			}
			private struct WlanReasonNotificationData
			{
				public Wlan.WlanNotificationData notifyData;
				public Wlan.WlanReasonCode reasonCode;
			}
			#endregion

			internal WlanInterface(WlanClient client, Wlan.WlanInterfaceInfo info)
			{
				this.client = client;
				this.info = info;
			}

			private void SetInterfaceInt(Wlan.WlanIntfOpcode opCode, int value)
			{
				IntPtr valuePtr = Marshal.AllocHGlobal(sizeof(int));
				Marshal.WriteInt32(valuePtr, value);
				try
				{
					Wlan.ThrowIfError(
						Wlan.WlanSetInterface(client.clientHandle, info.interfaceGuid, opCode, sizeof(int), valuePtr, IntPtr.Zero));
				}
				finally
				{
					Marshal.FreeHGlobal(valuePtr);
				}
			}

			private int GetInterfaceInt(Wlan.WlanIntfOpcode opCode)
			{
				IntPtr valuePtr;
				int valueSize;
				Wlan.WlanOpcodeValueType opcodeValueType;
				Wlan.ThrowIfError(
					Wlan.WlanQueryInterface(client.clientHandle, info.interfaceGuid, opCode, IntPtr.Zero, out valueSize, out valuePtr, out opcodeValueType));
				try
				{
					return Marshal.ReadInt32(valuePtr);
				}
				finally
				{
					Wlan.WlanFreeMemory(valuePtr);
				}
			}

			public bool Autoconf
			{
				get
				{
					return GetInterfaceInt(Wlan.WlanIntfOpcode.AutoconfEnabled) != 0;
				}
				set
				{
					SetInterfaceInt(Wlan.WlanIntfOpcode.AutoconfEnabled, value ? 1 : 0);
				}
			}

			public Wlan.Dot11BssType BssType
			{
				get
				{
					return (Wlan.Dot11BssType) GetInterfaceInt(Wlan.WlanIntfOpcode.BssType);
				}
				set
				{
					SetInterfaceInt(Wlan.WlanIntfOpcode.BssType, (int)value);
				}
			}

			public Wlan.WlanInterfaceState InterfaceState
			{
				get
				{
					return (Wlan.WlanInterfaceState)GetInterfaceInt(Wlan.WlanIntfOpcode.InterfaceState);
				}
			}

			public int Channel
			{
				get
				{
					return GetInterfaceInt(Wlan.WlanIntfOpcode.ChannelNumber);
				}
			}

			public int RSSI
			{
				get
				{
					return GetInterfaceInt(Wlan.WlanIntfOpcode.RSSI);
				}
			}

			public Wlan.Dot11OperationMode CurrentOperationMode
			{
				get
				{
					return (Wlan.Dot11OperationMode) GetInterfaceInt(Wlan.WlanIntfOpcode.CurrentOperationMode);
				}
			}

			public Wlan.WlanConnectionAttributes CurrentConnection
			{
				get
				{
					int valueSize;
					IntPtr valuePtr;
					Wlan.WlanOpcodeValueType opcodeValueType;
					Wlan.ThrowIfError(
						Wlan.WlanQueryInterface(client.clientHandle, info.interfaceGuid, Wlan.WlanIntfOpcode.CurrentConnection, IntPtr.Zero, out valueSize, out valuePtr, out opcodeValueType));
					try
					{
							return (Wlan.WlanConnectionAttributes)Marshal.PtrToStructure(valuePtr, typeof(Wlan.WlanConnectionAttributes));
					}
					finally
					{
						Wlan.WlanFreeMemory(valuePtr);
					}
				}
			}

			public void Scan()
			{
				Wlan.ThrowIfError(
					Wlan.WlanScan(client.clientHandle, info.interfaceGuid, IntPtr.Zero, IntPtr.Zero, IntPtr.Zero));
			}

			private Wlan.WlanAvailableNetwork[] ConvertAvailableNetworkListPtr(IntPtr availNetListPtr)
			{
				Wlan.WlanAvailableNetworkListHeader availNetListHeader = (Wlan.WlanAvailableNetworkListHeader)Marshal.PtrToStructure(availNetListPtr, typeof(Wlan.WlanAvailableNetworkListHeader));
				long availNetListIt = availNetListPtr.ToInt64() + Marshal.SizeOf(typeof(Wlan.WlanAvailableNetworkListHeader));
				Wlan.WlanAvailableNetwork[] availNets = new Wlan.WlanAvailableNetwork[availNetListHeader.numberOfItems];
				for (int i = 0; i < availNetListHeader.numberOfItems; ++i)
				{
					availNets[i] = (Wlan.WlanAvailableNetwork)Marshal.PtrToStructure(new IntPtr(availNetListIt), typeof(Wlan.WlanAvailableNetwork));
					availNetListIt += Marshal.SizeOf(typeof(Wlan.WlanAvailableNetwork));
				}
				return availNets;
			}

			public Wlan.WlanAvailableNetwork[] GetAvailableNetworkList(Wlan.WlanGetAvailableNetworkFlags flags)
			{
				IntPtr availNetListPtr;
				Wlan.ThrowIfError(
					Wlan.WlanGetAvailableNetworkList(client.clientHandle, info.interfaceGuid, flags, IntPtr.Zero, out availNetListPtr));
				try
				{
					return ConvertAvailableNetworkListPtr(availNetListPtr);
				}
				finally
				{
					Wlan.WlanFreeMemory(availNetListPtr);
				}
			}

			private Wlan.WlanBssEntry[] ConvertBssListPtr(IntPtr bssListPtr)
			{
				Wlan.WlanBssListHeader bssListHeader = (Wlan.WlanBssListHeader)Marshal.PtrToStructure(bssListPtr, typeof(Wlan.WlanBssListHeader));
				long bssListIt = bssListPtr.ToInt64() + Marshal.SizeOf(typeof(Wlan.WlanBssListHeader));
				Wlan.WlanBssEntry[] bssEntries = new Wlan.WlanBssEntry[bssListHeader.numberOfItems];
				for (int i=0; i<bssListHeader.numberOfItems; ++i)
				{
					bssEntries[i] = (Wlan.WlanBssEntry)Marshal.PtrToStructure(new IntPtr(bssListIt), typeof(Wlan.WlanBssEntry));
					bssListIt += Marshal.SizeOf(typeof(Wlan.WlanBssEntry));
				}
				return bssEntries;
			}

			public Wlan.WlanBssEntry[] GetNetworkBssList()
			{
				IntPtr bssListPtr;
				Wlan.ThrowIfError(
					Wlan.WlanGetNetworkBssList(client.clientHandle, info.interfaceGuid, IntPtr.Zero, Wlan.Dot11BssType.Any, false, IntPtr.Zero, out bssListPtr));
				try
				{
					return ConvertBssListPtr(bssListPtr);
				}
				finally
				{
					Wlan.WlanFreeMemory(bssListPtr);
				}
			}

			public Wlan.WlanBssEntry[] GetNetworkBssList(Wlan.Dot11Ssid ssid, Wlan.Dot11BssType bssType, bool securityEnabled)
			{
				IntPtr ssidPtr = Marshal.AllocHGlobal(Marshal.SizeOf(ssid));
				Marshal.StructureToPtr(ssid, ssidPtr, false);
				try
				{
					IntPtr bssListPtr;
					Wlan.ThrowIfError(
						Wlan.WlanGetNetworkBssList(client.clientHandle, info.interfaceGuid, ssidPtr, bssType, securityEnabled, IntPtr.Zero, out bssListPtr));
					try
					{
						return ConvertBssListPtr(bssListPtr);
					}
					finally
					{
						Wlan.WlanFreeMemory(bssListPtr);
					}
				}
				finally
				{
					Marshal.FreeHGlobal(ssidPtr);
				}
			}

			protected void Connect(Wlan.WlanConnectionParameters connectionParams)
			{
				Wlan.ThrowIfError(
					Wlan.WlanConnect(client.clientHandle, info.interfaceGuid, ref connectionParams, IntPtr.Zero));
			}

			public void Connect(Wlan.WlanConnectionMode connectionMode, Wlan.Dot11BssType bssType, string profile)
			{
				Wlan.WlanConnectionParameters connectionParams = new Wlan.WlanConnectionParameters();
				connectionParams.wlanConnectionMode = connectionMode;
				connectionParams.profile = profile;
				connectionParams.dot11BssType = bssType;
				connectionParams.flags = 0;
				Connect(connectionParams);
			}

			public bool ConnectSynchronously(Wlan.WlanConnectionMode connectionMode, Wlan.Dot11BssType bssType, string profile, int connectTimeout)
			{
				queueEvents = true;
				try
				{
					Connect(connectionMode, bssType, profile);
					while (queueEvents && eventQueueFilled.WaitOne(connectTimeout, true))
					{
						lock (eventQueue)
						{
							while (eventQueue.Count != 0)
							{
								object e = eventQueue.Dequeue();
								if (e is WlanConnectionNotificationEventData)
								{
									WlanConnectionNotificationEventData wlanConnectionData = (WlanConnectionNotificationEventData)e;
									// Check if the conditions are good to indicate either success or failure.
									if (wlanConnectionData.notifyData.notificationSource == Wlan.WlanNotificationSource.ACM)
									{
										switch ((Wlan.WlanNotificationCodeAcm)wlanConnectionData.notifyData.notificationCode)
										{
											case Wlan.WlanNotificationCodeAcm.ConnectionComplete:
												if (wlanConnectionData.connNotifyData.profileName == profile)
													return true;
												break;
										}
									}
									break;
								}
							}
						}
					}
				}
				finally
				{
					queueEvents = false;
					eventQueue.Clear();
				}
				return false; // timeout expired and no "connection complete"
			}

			public void Connect(Wlan.WlanConnectionMode connectionMode, Wlan.Dot11BssType bssType, Wlan.Dot11Ssid ssid, Wlan.WlanConnectionFlags flags)
			{
				Wlan.WlanConnectionParameters connectionParams = new Wlan.WlanConnectionParameters();
				connectionParams.wlanConnectionMode = connectionMode;
				connectionParams.dot11SsidPtr = Marshal.AllocHGlobal(Marshal.SizeOf(ssid));
				Marshal.StructureToPtr(ssid, connectionParams.dot11SsidPtr, false);
				connectionParams.dot11BssType = bssType;
				connectionParams.flags = flags;
				Connect(connectionParams);
				Marshal.DestroyStructure(connectionParams.dot11SsidPtr, ssid.GetType());
				Marshal.FreeHGlobal(connectionParams.dot11SsidPtr);
			}

			public void DeleteProfile(string profileName)
			{
				Wlan.ThrowIfError(
					Wlan.WlanDeleteProfile(client.clientHandle, info.interfaceGuid, profileName, IntPtr.Zero));
			}

			public Wlan.WlanReasonCode SetProfile(Wlan.WlanProfileFlags flags, string profileXml, bool overwrite)
			{
				Wlan.WlanReasonCode reasonCode;
				Wlan.ThrowIfError(
						Wlan.WlanSetProfile(client.clientHandle, info.interfaceGuid, flags, profileXml, null, overwrite, IntPtr.Zero, out reasonCode));
				return reasonCode;
			}

			public string GetProfileXml(string profileName)
			{
				IntPtr profileXmlPtr;
				Wlan.WlanProfileFlags flags;
				Wlan.WlanAccess access;
				Wlan.ThrowIfError(
					Wlan.WlanGetProfile(client.clientHandle, info.interfaceGuid, profileName, IntPtr.Zero, out profileXmlPtr, out flags,
					               out access));
				try
				{
					return Marshal.PtrToStringUni(profileXmlPtr);
				}
				finally
				{
					Wlan.WlanFreeMemory(profileXmlPtr);
				}
			}

			public Wlan.WlanProfileInfo[] GetProfiles()
			{
				IntPtr profileListPtr;
				Wlan.ThrowIfError(
					Wlan.WlanGetProfileList(client.clientHandle, info.interfaceGuid, IntPtr.Zero, out profileListPtr));
				try
				{
					Wlan.WlanProfileInfoListHeader header = (Wlan.WlanProfileInfoListHeader) Marshal.PtrToStructure(profileListPtr, typeof(Wlan.WlanProfileInfoListHeader));
					Wlan.WlanProfileInfo[] profileInfos = new Wlan.WlanProfileInfo[header.numberOfItems];
					long profileListIterator = profileListPtr.ToInt64() + Marshal.SizeOf(header);
					for (int i=0; i<header.numberOfItems; ++i)
					{
						Wlan.WlanProfileInfo profileInfo = (Wlan.WlanProfileInfo) Marshal.PtrToStructure(new IntPtr(profileListIterator), typeof(Wlan.WlanProfileInfo));
						profileInfos[i] = profileInfo;
						profileListIterator += Marshal.SizeOf(profileInfo);
					}
					return profileInfos;
				}
				finally
				{
					Wlan.WlanFreeMemory(profileListPtr);
				}
			}

			internal void OnWlanConnection(Wlan.WlanNotificationData notifyData, Wlan.WlanConnectionNotificationData connNotifyData)
			{
				if (WlanConnectionNotification != null)
					WlanConnectionNotification(notifyData, connNotifyData);

				if (queueEvents)
				{
					WlanConnectionNotificationEventData queuedEvent = new WlanConnectionNotificationEventData();
					queuedEvent.notifyData = notifyData;
					queuedEvent.connNotifyData = connNotifyData;
					EnqueueEvent(queuedEvent);
				}
			}

			internal void OnWlanReason(Wlan.WlanNotificationData notifyData, Wlan.WlanReasonCode reasonCode)
			{
				if (WlanReasonNotification != null)
					WlanReasonNotification(notifyData, reasonCode);
				if (queueEvents)
				{
					WlanReasonNotificationData queuedEvent = new WlanReasonNotificationData();
					queuedEvent.notifyData = notifyData;
					queuedEvent.reasonCode = reasonCode;
					EnqueueEvent(queuedEvent);
				}
			}

			internal void OnWlanNotification(Wlan.WlanNotificationData notifyData)
			{
				if (WlanNotification != null)
					WlanNotification(notifyData);
			}

			private void EnqueueEvent(object queuedEvent)
			{
				lock (eventQueue)
					eventQueue.Enqueue(queuedEvent);
				eventQueueFilled.Set();
			}

			public NetworkInterface NetworkInterface
			{
				get
				{
                    // Do not cache the NetworkInterface; We need it fresh
                    // each time cause otherwise it caches the IP information.
					foreach (NetworkInterface netIface in NetworkInterface.GetAllNetworkInterfaces())
					{
						Guid netIfaceGuid = new Guid(netIface.Id);
						if (netIfaceGuid.Equals(info.interfaceGuid))
						{
							return netIface;
						}
					}
                    return null;
				}
			}

			public Guid InterfaceGuid
			{
				get { return info.interfaceGuid; }
			}

			public string InterfaceDescription
			{
				get { return info.interfaceDescription; }
			}

			public string InterfaceName
			{
				get { return NetworkInterface.Name; }
			}
		}

		private IntPtr clientHandle;
		private uint negotiatedVersion;
		private Wlan.WlanNotificationCallbackDelegate wlanNotificationCallback;

		private Dictionary<Guid,WlanInterface> ifaces = new Dictionary<Guid,WlanInterface>();

		public WlanClient()
		{
			Wlan.ThrowIfError(
				Wlan.WlanOpenHandle(Wlan.WLAN_CLIENT_VERSION_XP_SP2, IntPtr.Zero, out negotiatedVersion, out clientHandle));
			try
			{
				Wlan.WlanNotificationSource prevSrc;
				wlanNotificationCallback = new Wlan.WlanNotificationCallbackDelegate(OnWlanNotification);
				Wlan.ThrowIfError(
					Wlan.WlanRegisterNotification(clientHandle, Wlan.WlanNotificationSource.All, false, wlanNotificationCallback, IntPtr.Zero, IntPtr.Zero, out prevSrc));
			}
			catch
			{
				Wlan.WlanCloseHandle(clientHandle, IntPtr.Zero);
				throw;
			}
		}

		~WlanClient()
		{
			Wlan.WlanCloseHandle(clientHandle, IntPtr.Zero);
		}

		private Wlan.WlanConnectionNotificationData? ParseWlanConnectionNotification(ref Wlan.WlanNotificationData notifyData)
		{
			int expectedSize = Marshal.SizeOf(typeof(Wlan.WlanConnectionNotificationData));
			if (notifyData.dataSize < expectedSize)
				return null;

			Wlan.WlanConnectionNotificationData connNotifyData =
				(Wlan.WlanConnectionNotificationData)
				Marshal.PtrToStructure(notifyData.dataPtr, typeof(Wlan.WlanConnectionNotificationData));
			if (connNotifyData.wlanReasonCode == Wlan.WlanReasonCode.Success)
			{
				IntPtr profileXmlPtr = new IntPtr(
					notifyData.dataPtr.ToInt64() +
					Marshal.OffsetOf(typeof(Wlan.WlanConnectionNotificationData), "profileXml").ToInt64());
				connNotifyData.profileXml = Marshal.PtrToStringUni(profileXmlPtr);
			}
			return connNotifyData;
		}

		private void OnWlanNotification(ref Wlan.WlanNotificationData notifyData, IntPtr context)
		{
			WlanInterface wlanIface = ifaces.ContainsKey(notifyData.interfaceGuid) ? ifaces[notifyData.interfaceGuid] : null;

			switch(notifyData.notificationSource)
			{
				case Wlan.WlanNotificationSource.ACM:
					switch((Wlan.WlanNotificationCodeAcm)notifyData.notificationCode)
					{
						case Wlan.WlanNotificationCodeAcm.ConnectionStart:
						case Wlan.WlanNotificationCodeAcm.ConnectionComplete:
						case Wlan.WlanNotificationCodeAcm.ConnectionAttemptFail:
						case Wlan.WlanNotificationCodeAcm.Disconnecting:
						case Wlan.WlanNotificationCodeAcm.Disconnected:
							Wlan.WlanConnectionNotificationData? connNotifyData = ParseWlanConnectionNotification(ref notifyData);
							if (connNotifyData.HasValue)
								if (wlanIface != null)
									wlanIface.OnWlanConnection(notifyData, connNotifyData.Value);
							break;
						case Wlan.WlanNotificationCodeAcm.ScanFail:
							{
								int expectedSize = Marshal.SizeOf(typeof (Wlan.WlanReasonCode));
								if (notifyData.dataSize >= expectedSize)
								{
									Wlan.WlanReasonCode reasonCode = (Wlan.WlanReasonCode) Marshal.ReadInt32(notifyData.dataPtr);
									if (wlanIface != null)
										wlanIface.OnWlanReason(notifyData, reasonCode);
								}
							}
							break;
					}
					break;
				case Wlan.WlanNotificationSource.MSM:
					switch((Wlan.WlanNotificationCodeMsm)notifyData.notificationCode)
					{
						case Wlan.WlanNotificationCodeMsm.Associating:
						case Wlan.WlanNotificationCodeMsm.Associated:
						case Wlan.WlanNotificationCodeMsm.Authenticating:
						case Wlan.WlanNotificationCodeMsm.Connected:
						case Wlan.WlanNotificationCodeMsm.RoamingStart:
						case Wlan.WlanNotificationCodeMsm.RoamingEnd:
						case Wlan.WlanNotificationCodeMsm.Disassociating:
						case Wlan.WlanNotificationCodeMsm.Disconnected:
						case Wlan.WlanNotificationCodeMsm.PeerJoin:
						case Wlan.WlanNotificationCodeMsm.PeerLeave:
						case Wlan.WlanNotificationCodeMsm.AdapterRemoval:
							Wlan.WlanConnectionNotificationData? connNotifyData = ParseWlanConnectionNotification(ref notifyData);
							if (connNotifyData.HasValue)
								if (wlanIface != null)
									wlanIface.OnWlanConnection(notifyData, connNotifyData.Value);
							break;
					}
					break;
			}

			if (wlanIface != null)
				wlanIface.OnWlanNotification(notifyData);
		}

		public WlanInterface[] Interfaces
		{
			get
			{
				IntPtr ifaceList;
				Wlan.ThrowIfError(
					Wlan.WlanEnumInterfaces(clientHandle, IntPtr.Zero, out ifaceList));
				try
				{
					Wlan.WlanInterfaceInfoListHeader header =
						(Wlan.WlanInterfaceInfoListHeader) Marshal.PtrToStructure(ifaceList, typeof (Wlan.WlanInterfaceInfoListHeader));
					Int64 listIterator = ifaceList.ToInt64() + Marshal.SizeOf(header);
					WlanInterface[] interfaces = new WlanInterface[header.numberOfItems];
					List<Guid> currentIfaceGuids = new List<Guid>();
					for (int i = 0; i < header.numberOfItems; ++i)
					{
						Wlan.WlanInterfaceInfo info =
							(Wlan.WlanInterfaceInfo) Marshal.PtrToStructure(new IntPtr(listIterator), typeof (Wlan.WlanInterfaceInfo));
						listIterator += Marshal.SizeOf(info);
						WlanInterface wlanIface;
						currentIfaceGuids.Add(info.interfaceGuid);
						if (ifaces.ContainsKey(info.interfaceGuid))
							wlanIface = ifaces[info.interfaceGuid];
						else
							wlanIface = new WlanInterface(this, info);
						interfaces[i] = wlanIface;
						ifaces[info.interfaceGuid] = wlanIface;
					}

					// Remove stale interfaces
					Queue<Guid> deadIfacesGuids = new Queue<Guid>();
					foreach (Guid ifaceGuid in ifaces.Keys)
					{
						if (!currentIfaceGuids.Contains(ifaceGuid))
							deadIfacesGuids.Enqueue(ifaceGuid);
					}
					while(deadIfacesGuids.Count != 0)
					{
						Guid deadIfaceGuid = deadIfacesGuids.Dequeue();
						ifaces.Remove(deadIfaceGuid);
					}

					return interfaces;
				}
				finally
				{
					Wlan.WlanFreeMemory(ifaceList);
				}
			}
		}

		public string GetStringForReasonCode(Wlan.WlanReasonCode reasonCode)
		{
			StringBuilder sb = new StringBuilder(1024); // the 1024 size here is arbitrary; the WlanReasonCodeToString docs fail to specify a recommended size
			Wlan.ThrowIfError(
				Wlan.WlanReasonCodeToString(reasonCode, sb.Capacity, sb, IntPtr.Zero));
			return sb.ToString();
		}
	}
}
'@

    function Convert-ByteArrayToString {
        [CmdletBinding()] Param (
            [Parameter(Mandatory = $True, ValueFromPipeline = $True)] [System.Byte[]] $ByteArray
            )

        $Encoding  = New-Object System.Text.ASCIIEncoding
        $Encoding.GetString($ByteArray)
    }

    Add-Type $NativeWifiCode
    $WlanClient = New-Object NativeWifi.WlanClient

    $WlanClient.Interfaces |
    ForEach-Object { $_.GetNetworkBssList() } |
    Select-Object *,@{Name="SSID";Expression={(Convert-ByteArrayToString -ByteArray $_.dot11ssid.SSID).substring(0,$_.dot11ssid.SSIDlength)}} |
    Select-Object ssid,phyId,rssi,linkQuality,timestamp
}
Get-BSSID
`
