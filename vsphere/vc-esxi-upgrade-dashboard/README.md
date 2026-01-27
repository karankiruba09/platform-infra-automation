# ESXi Upgrade Dashboard

A real-time web dashboard for monitoring ESXi host upgrades across multiple vCenter instances. Tracks upgrade progress, provides health status indicators, and enables centralized visibility into large-scale vSphere infrastructure upgrades.

## Features

- **Multi-vCenter Support**: Monitor upgrade progress across multiple vCenter instances simultaneously
- **Real-Time Progress Tracking**: Display total hosts, upgraded hosts, and completion percentage per vCenter
- **Health Status Indicators**: Color-coded status (Healthy, In Progress, At Risk) based on completion percentage
- **KPI Dashboard**: High-level metrics showing:
  - Number of connected vCenters
  - Total hosts across all vCenters
  - Total upgraded hosts
  - Overall weighted completion percentage
- **Auto-Refresh**: Automatic data refresh every 60 seconds
- **PNG Export**: Capture and download dashboard as PNG screenshot
- **Activity Log**: Real-time log of refresh events and errors
- **Responsive Design**: Works on desktop and tablet viewports

## Architecture

### Backend
- **`collector.py`**: Python script that:
  - Connects to each configured vCenter using PyVmomi (vSphere API)
  - Retrieves all ESXi hosts from the inventory
  - Compares host versions against target ESXi version
  - Generates aggregated JSON output with upgrade metrics

- **`api.py`**: Flask web server that:
  - Serves the dashboard UI (HTML, CSS, JavaScript)
  - Provides REST API endpoint `/api/v1/vcenters` to fetch upgrade data
  - Securely handles static file serving with path traversal protection

### Frontend
- **`index.html`**: Dashboard UI layout with KPI cards, status table, and activity log
- **`app.js`**: Client-side JavaScript that:
  - Fetches data from the API endpoint
  - Renders KPIs and progress bars with animations
  - Manages refresh cycles and error handling
  - Handles PNG export functionality
- **`style.css`**: Modern glassmorphic dark theme with gradient backgrounds, animations, and responsive layout

## Quick Start

Get the dashboard running in 5 minutes:

### 1. Set Up Virtual Environment

Create an isolated Python environment to avoid system-wide dependency conflicts:

**Linux/macOS:**
```bash
python3 -m venv venv
source venv/bin/activate
```

**Windows (PowerShell):**
```powershell
python -m venv venv
.\venv\Scripts\Activate.ps1
```

### 2. Install Dependencies

```bash
pip install -r requirements.txt
```

### 3. Configure vCenters

Edit `config/vcenters.yaml` with your vCenter details:

```yaml
vcenters:
  - name: vCenter1
    host: vcenter1.example.com
    user: administrator@vsphere.local
    password: "your-password"
  - name: vCenter2
    host: vcenter2.example.com
    user: administrator@vsphere.local
    password: "your-password"

target_esxi_version: "8.0.3"
```

### 4. Run Data Collector

Fetch ESXi host data from your vCenters:

```bash
python3 collector.py
```

You should see output like:
```
✓ vCenter1: 24 hosts
✓ vCenter2: 32 hosts

Wrote public/vcenters.json at 2026-01-27T14:32:45.123456+00:00
```

### 5. Start the Dashboard

```bash
python3 api.py
```

The dashboard will be available at: **http://localhost:8080**

### 6. Deactivate Virtual Environment (when done)

```bash
deactivate
```

---

## Prerequisites

- Python 3.6+
- Flask
- PyVmomi (VMware vSphere Python API)
- PyYAML
- Valid credentials for vCenter instances with appropriate read permissions

## Installation

1. Clone or download the project
2. Create and activate virtual environment (see Quick Start section)
3. Install Python dependencies:
   ```bash
   pip install -r requirements.txt
   ```

4. Configure vCenter connections in `config/vcenters.yaml`

## Configuration

Edit `config/vcenters.yaml` to specify your vCenter instances and target ESXi version:

```yaml
vcenters:
  - name: vCenter1
    host: vCenter1.example.com
    user: administrator@vsphere.local
    password: "your-password"
  - name: vCenter2
    host: vCenter2.example.com
    user: administrator@vsphere.local
    password: "your-password"
  - name: vCenter3
    host: vCenter3.example.com
    user: administrator@vsphere.local
    password: "your-password"

# Hosts are considered "upgraded" if they match this ESXi version exactly
target_esxi_version: "8.0.3"
```

**Security Note**: Consider using environment variables or a secrets management system instead of storing passwords in plaintext.

## Running the Application

### Option 1: Manual Collection + Web Server

1. **Run the data collector** to fetch current upgrade status from vCenters:
   ```bash
   python collector.py
   ```
   This generates `public/vcenters.json` with the latest metrics.

2. **Start the Flask web server**:
   ```bash
   python api.py
   ```
   The dashboard will be available at `http://localhost:8080`

### Option 2: Automated Collection (Recommended)

Schedule `collector.py` to run periodically using:
- **Linux/macOS**: cron job
- **Windows**: Task Scheduler
- **Container**: Docker container with scheduled runs

Then keep `api.py` running continuously to serve the dashboard.

Example cron job (every 5 minutes):
```bash
*/5 * * * * cd /path/to/vc-esxi-upgrade-dashboard && python collector.py
```

## Usage

1. Open the dashboard in your browser at `http://localhost:8080`
2. View upgrade progress across all vCenters
3. Click **Refresh** to manually trigger a data update
4. Click **Export PNG** to download a snapshot of the dashboard
5. Monitor the activity log for status updates and errors

## API Reference

### GET `/api/v1/vcenters`
Returns current upgrade metrics for all configured vCenters.

**Response Example**:
```json
{
  "generated_at": "2026-01-26T14:32:45.123456+00:00",
  "target_esxi_version": "8.0.3",
  "rows": [
    {
      "vcenter": "vCenter1",
      "total_hosts": 24,
      "upgrade_completed_total": 18,
      "completion_percentage": 75.0
    },
    {
      "vcenter": "vCenter2",
      "total_hosts": 32,
      "upgrade_completed_total": 32,
      "completion_percentage": 100.0
    }
  ]
}
```

## File Structure

```
vc-esxi-upgrade-dashboard/
├── api.py                    # Flask web server
├── collector.py              # vCenter data collector
├── requirements.txt          # Python dependencies
├── config/
│   └── vcenters.yaml        # vCenter configuration
├── public/
│   ├── index.html           # Dashboard HTML
│   ├── app.js               # Dashboard JavaScript
│   ├── style.css            # Dashboard styles
│   └── vcenters.json        # Generated metrics (created by collector.py)
└── README.md                # This file
```

## Health Status Logic

Progress bars and status indicators are color-coded based on completion percentage:

| Completion % | Status      | Color  | Label      |
|-------------|------------|--------|-----------|
| ≥ 99.9%    | Healthy    | Green  | ✓ Complete |
| 85% - 99.9% | In Progress | Orange | ⚠ In Progress |
| < 85%       | At Risk    | Red    | ✗ At Risk  |

## Development Notes

### Adding More Metrics

To track additional ESXi metrics (e.g., CPU model, memory, firmware):

1. Extend `build_vcenter_row()` in `collector.py` to retrieve additional properties
2. Update the JSON schema in the output
3. Modify the table in `index.html` and `app.js` to display new columns

### Performance Considerations

- Large vCenter environments may take time to enumerate all hosts
- Consider implementing pagination for tables with 100+ hosts
- Use connection pooling if connecting to many vCenters simultaneously

### SSL/TLS Verification

Currently, SSL verification is disabled (`insecure=True`) for development. For production:

```python
def connect_vcenter(host, user, password, port=443, insecure=False):
    context = None
    if insecure:
        context = ssl._create_unverified_context()
    # ... rest of function
```

Set `insecure=False` and provide proper SSL certificates.

## Error Handling

- Connection failures to a vCenter are logged but don't block other vCenters
- Missing `public/vcenters.json` on startup returns an error (run collector first)
- API errors are displayed in the activity log on the dashboard

## Future Enhancements

- Historical trend data and graphs
- Per-host upgrade status details
- Webhook notifications on upgrade milestones
- Database persistence for audit trails
- Authentication/authorization for API access
- Customizable target version per vCenter

## License

Check the repository's DISCLAIMER.md for licensing information.

## Support

For issues or questions about this project, refer to the repository documentation or contact the infrastructure team.
