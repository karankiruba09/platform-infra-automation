# Prompt for vCenter server name and credentials
$vcenterServer = Read-Host "Enter vCenter Server"
$vcUsername = Read-Host "Enter vCenter Username"
$vcPassword = Read-Host "Enter vCenter Password" -AsSecureString

# Connect to vCenter
Connect-VIServer -Server $vcenterServer -User $vcUsername -Password $vcPassword

# Get a list of all ESXi hosts
$esxiHosts = Get-VMHost

foreach ($esxihost in $esxiHosts) {
    $getesxcli = Get-Esxcli -VMhost $esxihost -V2

    # We can change this command to any esxi command we want to run
    $getesxcli.vsan.name
}

# Disconnect from vCenter
Disconnect-VIServer -Confirm:$false