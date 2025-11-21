#include <iostream>
#include <string>
#include <windows.h>
#include <tlhelp32.h>
#include <vector>

// Function to get the Process ID (PID) for a given process name
DWORD GetProcessIdByName(const wchar_t* processName) {
    PROCESSENTRY32W processEntry;
    processEntry.dwSize = sizeof(PROCESSENTRY32W);

    HANDLE hSnapshot = CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0);
    if (hSnapshot == INVALID_HANDLE_VALUE) {
        std::wcerr << L"[-] CreateToolhelp32Snapshot failed. Error: " << GetLastError() << std::endl;
        return 0;
    }

    if (Process32FirstW(hSnapshot, &processEntry)) {
        do {
            if (_wcsicmp(processEntry.szExeFile, processName) == 0) {
                CloseHandle(hSnapshot);
                return processEntry.th32ProcessID;
            }
        } while (Process32NextW(hSnapshot, &processEntry));
    }

    CloseHandle(hSnapshot);
    return 0;
}

// Suspends all threads in a process except the calling thread
void SuspendProcess(DWORD processId) {
    HANDLE hThreadSnapshot = CreateToolhelp32Snapshot(TH32CS_SNAPTHREAD, 0);
    if (hThreadSnapshot == INVALID_HANDLE_VALUE) {
        return;
    }

    THREADENTRY32 te32;
    te32.dwSize = sizeof(THREADENTRY32);
    DWORD currentThreadId = GetCurrentThreadId();

    if (Thread32First(hThreadSnapshot, &te32)) {
        do {
            if (te32.th32OwnerProcessID == processId && te32.th32ThreadID != currentThreadId) {
                HANDLE hThread = OpenThread(THREAD_SUSPEND_RESUME, FALSE, te32.th32ThreadID);
                if (hThread) {
                    SuspendThread(hThread);
                    CloseHandle(hThread);
                }
            }
        } while (Thread32Next(hThreadSnapshot, &te32));
    }
    CloseHandle(hThreadSnapshot);
}

// Resumes all threads in a process
void ResumeProcess(DWORD processId) {
    HANDLE hThreadSnapshot = CreateToolhelp32Snapshot(TH32CS_SNAPTHREAD, 0);
    if (hThreadSnapshot == INVALID_HANDLE_VALUE) {
        return;
    }

    THREADENTRY32 te32;
    te32.dwSize = sizeof(THREADENTRY32);

    if (Thread32First(hThreadSnapshot, &te32)) {
        do {
            if (te32.th32OwnerProcessID == processId) {
                HANDLE hThread = OpenThread(THREAD_SUSPEND_RESUME, FALSE, te32.th32ThreadID);
                if (hThread) {
                    ResumeThread(hThread);
                    CloseHandle(hThread);
                }
            }
        } while (Thread32Next(hThreadSnapshot, &te32));
    }
    CloseHandle(hThreadSnapshot);
}

int main() {
    const wchar_t* targetProcessName = L"Rome2.exe";
    const char* dllNameToFind = "memalochook.dll";

    // --- 1. Find Process ID ---
    std::wcout << L"[+] Searching for process: " << targetProcessName << std::endl;
    DWORD pid = GetProcessIdByName(targetProcessName);
    if (pid == 0) {
        std::wcerr << L"[-] Process not found. Is the game running?" << std::endl;
        system("pause");
        return 1;
    }
    std::wcout << L"[+] Process found! PID: " << pid << std::endl;

    // --- 2. Resolve DLL path (only in current directory) ---
    char dllPath[MAX_PATH];
    if (GetFullPathNameA(dllNameToFind, MAX_PATH, dllPath, nullptr) == 0) {
        std::cerr << "[-] Failed to get full path for DLL. Error: " << GetLastError() << std::endl;
        system("pause");
        return 1;
    }
    
    // Check if the file actually exists at the resolved path
    if (GetFileAttributesA(dllPath) == INVALID_FILE_ATTRIBUTES) {
        std::cerr << "[-] ERROR: " << dllNameToFind << " not found in the executable's directory." << std::endl;
        std::cerr << "    Expected at: " << dllPath << std::endl;
        std::cerr << "    Please ensure memalochook.dll is copied next to injector_attach.exe" << std::endl;
        system("pause");
        return 1;
    }
    std::cout << "[+] Found DLL at: " << dllPath << std::endl;


    // --- 3. Suspend & Inject ---
    std::wcout << L"[+] Suspending process..." << std::endl;
    SuspendProcess(pid);

    std::wcout << L"[+] Opening process handle..." << std::endl;
    DWORD desiredAccess = PROCESS_CREATE_THREAD | PROCESS_QUERY_INFORMATION | PROCESS_VM_OPERATION | PROCESS_VM_WRITE | PROCESS_VM_READ;
    HANDLE hProcess = OpenProcess(desiredAccess, FALSE, pid);
    if (!hProcess) {
        std::wcerr << L"[-] OpenProcess failed. Error: " << GetLastError() << std::endl;
        ResumeProcess(pid); // Ensure process is resumed even if OpenProcess fails
        system("pause");
        return 1;
    }

    LPVOID pRemoteMem = VirtualAllocEx(hProcess, NULL, strlen(dllPath) + 1, MEM_COMMIT | MEM_RESERVE, PAGE_READWRITE);
    if (!pRemoteMem) {
        std::cerr << "[-] VirtualAllocEx failed. Error: " << GetLastError() << std::endl;
        CloseHandle(hProcess);
        ResumeProcess(pid);
        system("pause");
        return 1;
    }

    if (!WriteProcessMemory(hProcess, pRemoteMem, dllPath, strlen(dllPath) + 1, NULL)) {
        std::cerr << "[-] WriteProcessMemory failed. Error: " << GetLastError() << std::endl;
        VirtualFreeEx(hProcess, pRemoteMem, 0, MEM_RELEASE);
        CloseHandle(hProcess);
        ResumeProcess(pid);
        system("pause");
        return 1;
    }

    HANDLE hRemoteThread = CreateRemoteThread(hProcess, NULL, 0, (LPTHREAD_START_ROUTINE)GetProcAddress(GetModuleHandleA("kernel32.dll"), "LoadLibraryA"), pRemoteMem, 0, NULL);
    if (!hRemoteThread) {
        std::cerr << "[-] CreateRemoteThread failed. Error: " << GetLastError() << std::endl;
        VirtualFreeEx(hProcess, pRemoteMem, 0, MEM_RELEASE);
        CloseHandle(hProcess);
        ResumeProcess(pid);
        system("pause");
        return 1;
    }

    std::cout << "[+] Injection thread created. Waiting for it to finish..." << std::endl;
    WaitForSingleObject(hRemoteThread, INFINITE);
    std::cout << "[+] DLL injected successfully." << std::endl;

    // --- 4. Cleanup and Resume ---
    VirtualFreeEx(hProcess, pRemoteMem, 0, MEM_RELEASE);
    CloseHandle(hRemoteThread);
    CloseHandle(hProcess);

    std::wcout << L"[+] Resuming process..." << std::endl;
    ResumeProcess(pid);

    std::wcout << L"\nInjection complete. Press any key to exit." << std::endl;
    system("pause");

    return 0;
}