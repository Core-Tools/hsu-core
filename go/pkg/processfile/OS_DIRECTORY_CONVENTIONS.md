# OS-Specific Directory Conventions Reference

This document explains the cross-platform directory management strategy implemented in `ProcessFileManager` for HSU Master applications across Windows, macOS, and Linux systems.

## 📋 **Overview**

The `ProcessFileManager` follows platform-specific conventions to ensure HSU Master integrates properly with each operating system's file system standards and user expectations.

### **File Types Managed**
- **PID Files** - Process identification files (`.pid`)
- **Port Files** - Network port tracking files (`.port`) 
- **Log Files** - Application and worker log output (`.log`)

### **Service Contexts**
- **SystemService** - System-wide daemons/services
- **UserService** - User-specific services  
- **SessionService** - Session-scoped temporary services

---

## 🪟 **Windows Directory Conventions**

### **System Services** (`SystemService`)
| File Type | Base Directory | Example Path |
|-----------|---------------|--------------|
| **PID/Port** | `%PROGRAMDATA%` | `C:\ProgramData\hsu-master\worker1.pid` |
| **Logs** | `%PROGRAMDATA%` | `C:\ProgramData\hsu-master\logs\aggregated.log` |

### **User Services** (`UserService`)  
| File Type | Base Directory | Example Path |
|-----------|---------------|--------------|
| **PID/Port** | `%LOCALAPPDATA%` | `C:\Users\John\AppData\Local\hsu-master\worker1.pid` |
| **Logs** | `%LOCALAPPDATA%\logs` | `C:\Users\John\AppData\Local\logs\hsu-master\worker1.log` |

### **Session Services** (`SessionService`)
| File Type | Base Directory | Example Path |
|-----------|---------------|--------------|
| **PID/Port** | `%TEMP%` | `C:\Users\John\AppData\Local\Temp\worker1.pid` |
| **Logs** | `%TEMP%\logs` | `C:\Users\John\AppData\Local\Temp\logs\worker1.log` |

### **Windows Environment Variables**
- `%PROGRAMDATA%` - Usually `C:\ProgramData`
- `%LOCALAPPDATA%` - Usually `C:\Users\{Username}\AppData\Local`
- `%TEMP%` - Usually `C:\Users\{Username}\AppData\Local\Temp`

---

## 🍎 **macOS Directory Conventions**

### **System Services** (`SystemService`)
| File Type | Base Directory | Example Path |
|-----------|---------------|--------------|
| **PID/Port** | `/var/run` | `/var/run/hsu-master/worker1.pid` |
| **Logs** | `/var/log` | `/var/log/hsu-master/worker1.log` |

### **User Services** (`UserService`)
| File Type | Base Directory | Example Path |
|-----------|---------------|--------------|
| **PID/Port** | `~/Library/Application Support` | `~/Library/Application Support/hsu-master/worker1.pid` |
| **Logs** | `~/Library/Logs` | `~/Library/Logs/hsu-master/worker1.log` |

### **Session Services** (`SessionService`)
| File Type | Base Directory | Example Path |
|-----------|---------------|--------------|
| **PID/Port** | `/tmp` | `/tmp/worker1.pid` |
| **Logs** | `/tmp/logs` | `/tmp/logs/worker1.log` |

### **macOS Directory Rationale**
- **Application Support** vs **Logs** separation follows [Apple File System Programming Guide](https://developer.apple.com/library/archive/documentation/FileManagement/Conceptual/FileSystemProgrammingGuide/FileSystemOverview/FileSystemOverview.html)
- **Application Support** - Configuration, PID files, databases, runtime state
- **Logs** - Diagnostic output, audit trails, debugging information

---

## 🐧 **Linux Directory Conventions**

### **System Services** (`SystemService`)
| File Type | Base Directory | Example Path |
|-----------|---------------|--------------|
| **PID/Port** | `/run` (fallback `/var/run`) | `/run/hsu-master/worker1.pid` |
| **Logs** | `/var/log` | `/var/log/hsu-master/worker1.log` |

### **User Services** (`UserService`)
| File Type | Base Directory | Example Path |
|-----------|---------------|--------------|
| **PID/Port** | `$XDG_RUNTIME_DIR` (fallback `/tmp`) | `/run/user/1000/worker1.pid` |
| **Logs** | `$XDG_DATA_HOME/logs` (fallback `~/.local/share/logs`) | `~/.local/share/logs/hsu-master/worker1.log` |

### **Session Services** (`SessionService`)
| File Type | Base Directory | Example Path |
|-----------|---------------|--------------|
| **PID/Port** | `/run/user/$UID` (fallback `/tmp`) | `/run/user/1000/worker1.pid` |
| **Logs** | `/run/user/$UID/logs` (fallback `/tmp/logs`) | `/run/user/1000/logs/worker1.log` |

### **Linux Standards Compliance**
- **[XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html)** - Modern Linux standard
- **systemd** - `/run` for runtime data, `/run/user/$UID` for user sessions
- **FHS (Filesystem Hierarchy Standard)** - `/var/log` for system logs

---

## 🏗️ **Implementation Details**

### **Directory Resolution Logic**
```go
// 1. Check explicit BaseDirectory configuration
if config.BaseDirectory != "" {
    return config.BaseDirectory
}

// 2. Use service context to determine OS-appropriate path
switch config.ServiceContext {
case SystemService: return getSystemServiceDirectory()
case UserService:   return getUserServiceDirectory() 
case SessionService: return getSessionServiceDirectory()
}
```

### **Subdirectory Structure**
When `UseSubdirectory: true` (recommended for production):
```
Base Directory/
├── {AppName}/           # e.g., "hsu-master"
│   ├── worker1.pid
│   ├── worker2.pid
│   └── logs/
│       ├── aggregated.log
│       └── workers/
│           ├── worker1.log
│           └── worker2.log
```

### **Environment Variable Fallbacks**
Each platform implements robust fallback mechanisms:
- **Windows**: Hardcoded paths if environment variables missing
- **macOS**: `/tmp` fallback for user directory resolution failures  
- **Linux**: Multiple XDG variables with traditional Unix fallbacks

---

## 🎯 **Best Practices & Recommendations**

### **Production Deployments**
- ✅ **Use `SystemService`** for production system daemons
- ✅ **Set `UseSubdirectory: true`** to avoid file conflicts
- ✅ **Monitor disk space** in system directories
- ✅ **Implement log rotation** for long-running services

### **Development & Testing**
- ✅ **Use `UserService`** for local development
- ✅ **Use "development" scenario** for isolated testing
- ✅ **Use `SessionService`** for temporary testing

### **Cross-Platform Considerations**
- 🔒 **Permissions** - System directories require elevated privileges
- 📁 **Path Separators** - `filepath.Join()` handles OS differences automatically
- 🧹 **Cleanup** - Session services auto-cleanup on logout/reboot
- 🔍 **Monitoring** - Different tools on each platform expect standard locations

---

## 📚 **Configuration Examples**

### **System Service Configuration**
```go
config := ProcessFileConfig{
    ServiceContext:  SystemService,
    AppName:         "hsu-master",
    UseSubdirectory: true,
}
```
**Results in:**
- Windows: `C:\ProgramData\hsu-master\`
- macOS: `/var/run/hsu-master/`  
- Linux: `/run/hsu-master/`

### **User Service Configuration**
```go
config := ProcessFileConfig{
    ServiceContext:  UserService,
    AppName:         "hsu-master",
    UseSubdirectory: true,
}
```
**Results in:**
- Windows: `%LOCALAPPDATA%\hsu-master\`
- macOS: `~/Library/Application Support/hsu-master/`
- Linux: `$XDG_RUNTIME_DIR/hsu-master/` or `~/.local/share/hsu-master/`

### **Development Configuration**
```go
config := GetRecommendedProcessFileConfig("development", "hsu-master")
```
**Results in:**
- All platforms: `{temp}/hsu-master-dev/`

---

## 🔍 **Platform Standards References**

### **Windows**
- [Known Folders (Microsoft)](https://docs.microsoft.com/en-us/windows/win32/shell/knownfolderid)
- [Environment Variables (Microsoft)](https://docs.microsoft.com/en-us/windows/deployment/usmt/usmt-recognized-environment-variables)

### **macOS**  
- [File System Programming Guide (Apple)](https://developer.apple.com/library/archive/documentation/FileManagement/Conceptual/FileSystemProgrammingGuide/)
- [Bundle Programming Guide (Apple)](https://developer.apple.com/library/archive/documentation/CoreFoundation/Conceptual/CFBundles/)

### **Linux**
- [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html)
- [Filesystem Hierarchy Standard (FHS)](https://refspecs.linuxfoundation.org/FHS_3.0/fhs/index.html)
- [systemd File Hierarchy](https://www.freedesktop.org/software/systemd/man/file-hierarchy.html)

---

## 🚀 **Summary**

The `ProcessFileManager` implements a sophisticated cross-platform directory management system that:

- ✅ **Follows native conventions** on each operating system
- ✅ **Separates concerns** between application data and logs  
- ✅ **Supports multiple deployment contexts** (system/user/session)
- ✅ **Provides robust fallbacks** for missing environment variables
- ✅ **Enables proper permissions management** across platforms
- ✅ **Facilitates system administration** by using expected locations

This ensures HSU Master applications feel "native" on each platform and integrate seamlessly with existing system administration workflows and monitoring tools. 