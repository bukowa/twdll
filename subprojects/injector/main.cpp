#include <iostream>
#include <string>
#include <vector>
#include <windows.h>

int wmain(int argc, wchar_t* argv[])
{
    if (argc != 3) {
        std::wcerr << L"[-] Błąd: Nieprawidłowa liczba argumentów." << std::endl;
        std::wcerr << L"    Użycie: " << argv[0] << L" \"<pełna_ścieżka_do_gry.exe>\" \"<pełna_ścieżka_do_twojego.dll>\"" << std::endl;
        system("pause");
        return 1;
    }

    // --- Krok 1: Pobierz ścieżki z argumentów ---
    std::wstring targetProcessPath = argv[1];
    const wchar_t* dllPath = argv[2];

    // --- Krok 2: WYEKSTRAHUJ KATALOG ROBOCZY ZE ŚCIEŻKI DO GRY ---
    // To jest kluczowa poprawka, której brakowało.
    std::wstring workingDirectory = L"";
    size_t lastSlash = targetProcessPath.find_last_of(L"\\/");
    if (std::string::npos != lastSlash) {
        workingDirectory = targetProcessPath.substr(0, lastSlash);
    }
    
    if (workingDirectory.empty()) {
        std::wcerr << L"[!] Błąd: Nie można było ustalić katalogu roboczego ze ścieżki: " << targetProcessPath << std::endl;
        system("pause");
        return 1;
    }

    std::wcout << L"[+] Uruchamianie procesu:" << std::endl;
    std::wcout << L"    Aplikacja: " << targetProcessPath.c_str() << std::endl;
    std::wcout << L"    Katalog roboczy: " << workingDirectory.c_str() << std::endl;

    STARTUPINFOW si = { sizeof(si) };
    PROCESS_INFORMATION pi = { 0 };

    // --- Krok 3: Uruchom proces z PRAWIDŁOWYM katalogiem roboczym ---
    if (!CreateProcessW(
        (LPWSTR)targetProcessPath.c_str(), // Ścieżka do aplikacji
        NULL,                              // Bez argumentów
        NULL,
        NULL,
        FALSE,
        CREATE_SUSPENDED,                  // Używamy prostej i sprawdzonej flagi
        NULL,
        workingDirectory.c_str(),          // <<<< KRYTYCZNA POPRAWKA >>>>
        &si,
        &pi)
    ) {
        std::cerr << "[!] CreateProcess nie powiodło się. Kod błędu: " << GetLastError() << std::endl;
        system("pause");
        return 1;
    }

    // --- Reszta kodu injectora, który już znamy ---
    std::wcout << L"[+] Proces utworzony w stanie wstrzymanym. PID: " << pi.dwProcessId << std::endl;

    size_t dllPathSize = (wcslen(dllPath) + 1) * sizeof(wchar_t);
    LPVOID remoteMemory = VirtualAllocEx(pi.hProcess, NULL, dllPathSize, MEM_COMMIT | MEM_RESERVE, PAGE_READWRITE);
    if (!remoteMemory) { std::cerr << "[!] VirtualAllocEx FAILED" << std::endl; TerminateProcess(pi.hProcess, 1); return 1; }

    if (!WriteProcessMemory(pi.hProcess, remoteMemory, dllPath, dllPathSize, NULL)) { std::cerr << "[!] WriteProcessMemory FAILED" << std::endl; TerminateProcess(pi.hProcess, 1); return 1; }
    
    LPTHREAD_START_ROUTINE pLoadLibraryW = (LPTHREAD_START_ROUTINE)GetProcAddress(GetModuleHandleW(L"kernel32.dll"), "LoadLibraryW");
    HANDLE hRemoteThread = CreateRemoteThread(pi.hProcess, NULL, 0, pLoadLibraryW, remoteMemory, 0, NULL);
    if (!hRemoteThread) { std::cerr << "[!] CreateRemoteThread FAILED" << std::endl; TerminateProcess(pi.hProcess, 1); return 1; }

    WaitForSingleObject(hRemoteThread, INFINITE);
    std::wcout << L"[+] DLL wstrzyknięty pomyślnie." << std::endl;
    
    std::wcout << L"[+] Wznawianie procesu..." << std::endl;
    ResumeThread(pi.hThread);

    CloseHandle(hRemoteThread);
    CloseHandle(pi.hProcess);
    CloseHandle(pi.hThread);
    VirtualFreeEx(pi.hProcess, remoteMemory, 0, MEM_RELEASE);

    std::cout << "\nOperacja zakończona." << std::endl;
    Sleep(2000);

    return 0;
}