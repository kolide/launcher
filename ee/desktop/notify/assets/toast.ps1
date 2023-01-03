[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.UI.Notifications.ToastNotification, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null

$toast = @"
<toast duration="Short">
    <visual>
        <binding template="ToastGeneric">
            <text><![CDATA[{{.Title}}]]></text>
            <text><![CDATA[{{.Body}}]]></text>
        </binding>
    </visual>
	<audio silent="true" />
</toast>
"@

$toastXml = New-Object Windows.Data.Xml.Dom.XmlDocument
$toastXml.LoadXml($toast)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("Kolide Launcher").Show($toastXml)
