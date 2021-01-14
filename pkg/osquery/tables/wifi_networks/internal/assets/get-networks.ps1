function Get-Networks {
    function Convert-ByteArrayToString {
        [CmdletBinding()] Param (
            [Parameter(Mandatory = $True, ValueFromPipeline = $True)] [System.Byte[]] $ByteArray
            )

        $Encoding  = New-Object System.Text.ASCIIEncoding
        $Encoding.GetString($ByteArray)
    }

    Add-Type -Path ".\nativewificode.cs"
    $WlanClient = New-Object NativeWifi.WlanClient

    $WlanClient.Interfaces | ForEach-Object { $_.Scan() }

    # check scan progress for each interface
    $scanInProgress = "false"
    do {
      $scanInProgress = "false"
      $WlanClient.Interfaces | ForEach-Object {
        $ip = $_.scanInProgress
        if ($ip -eq "True") {
          $scanInProgress = "true"
        }
      };
      Start-Sleep -Milliseconds 100
    } while ($scanInProgress -eq "true")

    $WlanClient.Interfaces |
    ForEach-Object { $_.GetNetworkBssList() } |
    Select-Object *,@{Name="SSID";Expression={(Convert-ByteArrayToString -ByteArray $_.dot11ssid.SSID).substring(0,$_.dot11ssid.SSIDlength)}},
                    @{Name="BSSID";Expression={[System.BitConverter]::ToString($_.dot11Bssid) }} |
                    ConvertTo-Json 
}
Get-Networks 
