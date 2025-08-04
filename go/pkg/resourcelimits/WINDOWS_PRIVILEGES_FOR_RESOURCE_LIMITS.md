# References and Best Practices for Setting Resource Limits with Required Windows Privileges

To set resource limits (memory, CPU, and process limits) in Windows using Job Objects, the process needs to hold either `SeIncreaseQuotaPrivilege` or `SeDebugPrivilege` privilege.

It is **not possible** to programmatically grant or enable `SeIncreaseQuotaPrivilege` or `SeDebugPrivilege` purely at runtime in code. These privileges are controlled by Windows security policies and require administrative intervention.

---

## Summary of Key Points

- **`SeIncreaseQuotaPrivilege` ("Adjust memory quotas for a process")** and **`SeDebugPrivilege` ("Debug programs")** are typically assigned to the **Administrators** group by default on Windows.
- These privileges are **NOT always enabled by default**, even for Administrator accounts, and often need manual enabling via system administration tools.
- Ordinary user accounts **do not have these privileges** and cannot elevate themselves in code without appropriate rights.
- To enable the "set resource limits" feature (memory, CPU, process limits), users must:
  - Run the program under an account with these privileges assigned.
  - Verify and adjust privilege assignment in **Local Security Policy** or via **Group Policy**.
  - The program itself should enable the privileges in its token during startup using `AdjustTokenPrivileges`.

---

## Administrative Setup Instructions for Customers

1. **Run as Administrator**  
   Launch the program elevated with Administrator credentials.

2. **Verify Privileges with `whoami`**  
- Open a command prompt and run:
  ```
  whoami /priv
  ```
- Look for the privileges `SeIncreaseQuotaPrivilege` and `SeDebugPrivilege` and check if they are "Enabled".

3. **Adjust Local Security Policy (`secpol.msc`)**  
- Run `secpol.msc`.
- Navigate to:
  ```
  Security Settings -> Local Policies -> User Rights Assignment
  ```
- Check and edit the following policies:
  - **"Adjust memory quotas for a process"** (for `SeIncreaseQuotaPrivilege`)
  - **"Debug programs"** (for `SeDebugPrivilege`)
- Add the user account or relevant user groups if missing.

4. **Domain Environment Note**  
If controlled by Active Directory Group Policies (GPO), a Domain Administrator may need to modify GPO settings accordingly.

5. **Programmatic Enabling in Your Application**  
After running with sufficient rights, your program should enable the privileges explicitly in its access token using Windows API calls like `AdjustTokenPrivileges`.

---

## Additional Notes

- These privileges allow powerful operations; hence they are restricted to minimize security risks.
- Proper assignment and enabling of these privileges is crucial to avoid errors like:
  > "current process does not hold privileges for memory limits"
  > "current process does not hold privileges for CPU limits"
- Always encourage customers to operate under the principle of least privilege while granting necessary rights.
- The HSU Master process automatically detects missing privileges and provides clear error messages.
- Detailed developer and sysadmin documentation is available on Microsoft Docs regarding privileges and job object resource quotas.

---

## References

- Microsoft Docs: [Debug programs (SeDebugPrivilege)](https://learn.microsoft.com/en-us/windows/security/threat-protection/security-policy-settings/debug-programs)
- Microsoft Docs: [Adjust memory quotas for a process (SeIncreaseQuotaPrivilege)](https://learn.microsoft.com/en-us/windows/security/threat-protection/security-policy-settings/adjust-memory-quotas-for-a-process)
- Local Security Policy editor (`secpol.msc`)
- `whoami /priv` command-line tool to query current user privileges
- Windows API: [`AdjustTokenPrivileges`](https://learn.microsoft.com/en-us/windows/win32/api/securitybaseapi/nf-securitybaseapi-adjusttokenprivileges)

