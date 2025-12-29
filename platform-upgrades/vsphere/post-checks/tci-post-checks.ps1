param(
    [Parameter(Mandatory=$true)]
    [string]$vcenter,

    [Parameter(Mandatory=$true)]
    [System.IO.FileInfo]$vcCredential,

    [Parameter(Mandatory=$true)]
    [SecureString]$esxiPassword,   # just the password, user is always root

    [Parameter(Mandatory=$false)]
    [string]$cluster
)

# Import vCenter credential
$vcCreds = Import-Clixml -Path $vcCredential.FullName

# Build ESXi credential (always root)
$secPass   = ConvertTo-SecureString $esxiPassword -AsPlainText -Force
$esxiCreds = New-Object System.Management.Automation.PSCredential ("root",$secPass)

# Connect to vCenter
Connect-VIServer -Server $vcenter -Credential $vcCreds

# Define output file in current working directory
$outputFile = Join-Path -Path $PWD -ChildPath "ESXi_Report.html"

# Initialize HTML report
$html = @"
<html>
<head>
<style>
body { font-family: Arial, sans-serif; }
h2 { color: #2e6c80; }
table { border-collapse: collapse; width: 100%; margin-bottom: 20px; }
th, td { border: 1px solid #ccc; padding: 6px; text-align: left; }
th { background-color: #f2f2f2; }
.bad { background-color: #ffcccc; }
.good { background-color: #ccffcc; }
</style>
</head>
<body>
<h1>ESXi Host NIC & VIB Report</h1>
"@

# Get hosts depending on cluster parameter, then sort by Name
if ($cluster) {
    Write-Host "Collecting hosts from cluster: $cluster"
    $vmhosts = Get-Cluster -Name $cluster | Get-VMHost | Sort-Object Name
} else {
    Write-Host "Collecting hosts from entire vCenter"
    $vmhosts = Get-VMHost | Sort-Object Name
}

foreach ($vmhost in $vmhosts) {
    Write-Host "Processing $vmhost..."

    # Enable SSH service
    $sshService = Get-VMHostService -VMHost $vmhost | Where-Object {$_.Key -eq "TSM-SSH"}
    Start-VMHostService -HostService $sshService -Confirm:$false

    try {
        # Create SSH session to the host
        $session = New-SSHSession -ComputerName $vmhost.Name -Credential $esxiCreds -AcceptKey

        # Run commands using the session
        $vib_output = Invoke-SSHCommand -SessionId $session.SessionId -Command "esxcli software vib list | grep -E '^(smartpqi|i40en)[[:space:]]'"
        $nic_output = Invoke-SSHCommand -SessionId $session.SessionId -Command "esxcli network nic get -n vmnic5 | grep Driver -A 2"

        # Extract firmware version
        $fw_version = ($nic_output.Output -split "`n" | Where-Object {$_ -match "Firmware Version"}) -replace ".*Firmware Version:\s*", ""
        $fwClass = if ($fw_version -match "^9\.30") { "good" } else { "bad" }

        # Append host section
        $html += "<h2>Host: $vmhost</h2>"
        $html += "<h3>VIBs (smartpqi / i40en)</h3><pre>$([System.Web.HttpUtility]::HtmlEncode($vib_output.Output))</pre>"

        $html += "<h3>NIC vmnic5</h3><table>"
        foreach ($line in ($nic_output.Output -split "`n")) {
            if ($line -match ":") {
                $key,$val = $line -split ":",2
                $key = $key.Trim()
                $val = $val.Trim()
                if ($key -eq "Firmware Version") {
                    $html += "<tr><td>$key</td><td class='$fwClass'>$val</td></tr>"
                } else {
                    $html += "<tr><td>$key</td><td>$val</td></tr>"
                }
            }
        }
        $html += "</table>"
    }
    finally {
        # Always close SSH session
        if ($session) { Remove-SSHSession -SessionId $session.SessionId }
        # Stop SSH service again
        Stop-VMHostService -HostService $sshService -Confirm:$false
    }
}

# Close HTML
$html += "</body></html>"

# Save report
Set-Content -Path $outputFile -Value $html -Encoding UTF8

Write-Host "HTML report saved to $outputFile"