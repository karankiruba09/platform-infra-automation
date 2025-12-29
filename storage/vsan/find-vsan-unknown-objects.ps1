[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string] $vCenter,

    [Parameter(Mandatory = $true)]
    [string] $Cluster,

    [Parameter(Mandatory = $true)]
    [PSCredential] $Credential
)

$ErrorActionPreference = 'Stop'

# --- Handle credential input ---
if ($Credential -is [string]) {
    if (Test-Path $Credential) {
        try {
            Write-Verbose "Importing credential from file: $Credential"
            $Credential = Import-Clixml -Path $Credential
        }
        catch {
            throw "Failed to import credential file '$Credential'. Ensure it was created with Export-Clixml under your account."
        }
    }
    else {
        throw "Credential parameter is a string but not a valid file path: $Credential"
    }
}
elseif (-not ($Credential -is [pscredential])) {
    throw "Credential must be either a PSCredential object or a path to a saved credential file."
}

# --- PowerCLI configuration ---
Set-PowerCLIConfiguration -Scope User -ParticipateInCEIP $false -Confirm:$false | Out-Null
Set-PowerCLIConfiguration -InvalidCertificateAction Ignore -Confirm:$false | Out-Null

# --- Connect to vCenter ---
Connect-VIServer -Server $vCenter -Credential $Credential | Out-Null

# --- Identify Unassociated vSAN objects ---
$clusterView  = Get-Cluster -Name $Cluster
if (-not $clusterView) {
    throw "Cluster '$Cluster' not found."
}

$ClusterMoRef = $clusterView.ExtensionData.MoRef
$vmhost       = ($clusterView | Get-VMHost | Select-Object -First 1)
if (-not $vmhost) {
    throw "No ESXi hosts found in cluster '$Cluster'."
}

$vsanIntSys   = Get-View $vmhost.ExtensionData.configManager.vsanInternalSystem
$vsanClusterObjectSys = Get-VsanView -Id VsanObjectSystem-vsan-cluster-object-system

$results = ($vsanClusterObjectSys.VsanQueryObjectIdentities(
                $ClusterMoRef, $null, $null, $true, $true, $false
            ).Identities | Where-Object { $_.Vm -eq $null })

# --- Prepare collections ---
$detailedResults = @()
$summaryResults  = @()
$deletionCommands = @()

foreach ($result in $results) {
    $jsonResult = ($vsanIntSys.GetVsanObjExtAttrs($result.Uuid)) | ConvertFrom-Json

    # Detailed output
    foreach ($object in $jsonResult | Get-Member -MemberType NoteProperty) {
        $objectID = $object.Name
        $record   = $jsonResult.$objectID | Select-Object *
        Add-Member -InputObject $record -NotePropertyName "ClusterName" -NotePropertyValue $Cluster
        $detailedResults += $record
    }

    # Summary output: extract user friendly name from nested object
    $userFriendlyName = $null
    foreach ($object in $jsonResult | Get-Member -MemberType NoteProperty) {
        $props = $jsonResult.$($object.Name)
        if ($props.PSObject.Properties.Name -contains 'User friendly name') {
            $userFriendlyName = $props.'User friendly name'
            break
        }
    }

    $summaryResults += [PSCustomObject]@{
        ClusterName          = $Cluster
        UUID                 = $result.Uuid
        'User friendly name' = $userFriendlyName
    }

    # Deletion command
    $deletionCommands += "/usr/lib/vmware/osfs/bin/objtool delete -u $($result.Uuid) -f"
}

# --- Save to files ---
$detailedFile = "vsan_detailed_output_${Cluster}.txt"
$summaryFile  = "vsan_summary_output_${Cluster}.csv"
$deletionFile = "vsan_deletion_commands_${Cluster}.txt"

$detailedResults | Out-File -FilePath $detailedFile
$summaryResults  | Export-Csv -Path $summaryFile -NoTypeInformation
$deletionCommands | Out-File -FilePath $deletionFile

Write-Host "`n✅ Detailed output saved to: $detailedFile"
Write-Host "✅ Summary output saved to:  $summaryFile"
Write-Host "✅ Deletion commands saved to: $deletionFile`n"