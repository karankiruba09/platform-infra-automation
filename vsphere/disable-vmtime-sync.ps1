<#
.SYNOPSIS
  Connects to vCenter, finds VMs by name pattern, and disables periodic VMware Tools time sync.

.DESCRIPTION
  - Imports PSCredential from a secure XML file.
  - Connects to the specified vCenter Server.
  - Outputs the connected vCenter name.
  - Retrieves VMs matching the name pattern.
  - Uses the vSphere API to set tools.syncTimeWithHost = $false (un-checks periodic sync).
  - Reports success for each VM.

.PARAMETER CredentialFile
  Path to the Export-Clixml credential file.

.PARAMETER VCServer
  Hostname or IP of the vCenter Server.

.PARAMETER NamePattern
  Wildcard pattern for VM names (e.g., 'web-*').

.EXAMPLE
  .\Disable-VMTimeSync.ps1 -CredentialFile 'C:\Secure\vcCred.xml' -VCServer 'vcenter.example.com' -NamePattern 'app-*'
#>

param (
    [Parameter(Mandatory = $true)]
    [System.IO.FileInfo] $CredentialFile,

    [Parameter(Mandatory = $true)]
    [string] $VCServer,

    [Parameter(Mandatory = $true)]
    [string] $NamePattern
)

# 1. Import stored credentials
$cred = Import-Clixml -Path $CredentialFile
Write-Host "Credentials loaded from $CredentialFile" -ForegroundColor Green

# 2. Connect to vCenter
$vcConn = Connect-VIServer -Server $VCServer -Credential $cred -ErrorAction Stop
Write-Host "Connected to vCenter:" $vcConn.Name -ForegroundColor Cyan

# 3. Retrieve VMs matching the pattern
$vmList = Get-VM -Name $NamePattern
if ($vmList.Count -eq 0) {
    Write-Warning "No VMs found matching pattern '$NamePattern'"
    exit 1
}
Write-Host "Found $($vmList.Count) VM(s) matching '$NamePattern'" -ForegroundColor Green

# 4. Prepare the VM reconfiguration spec
$spec = New-Object VMware.Vim.VirtualMachineConfigSpec
$spec.tools = New-Object VMware.Vim.ToolsConfigInfo
$spec.tools.syncTimeWithHost = $false  # Disable periodic time sync :contentReference[oaicite:2]{index=2}

# 5. Apply to each VM
foreach ($vm in $vmList) {
    $vmView = Get-View -Id $vm.Id
    $vmView.ReconfigVM($spec)
    Write-Host "Disabled periodic time sync for VM:" $vm.Name
}

# 6. Disconnect
Disconnect-VIServer -Server $vcConn -Confirm:$false
Write-Host "Disconnected from $($vcConn.Name)" -ForegroundColor Cyan
