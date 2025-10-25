// ----------------------------------------------------------------------------------
// FILE: dx_finder.cpp
// PURPOSE: Reliably hooks D3D11 Present and ResizeBuffers for rendering.
// This version contains the focused fix for the WndProc crash during renderer resets.
// ----------------------------------------------------------------------------------

#include "dx_finder.h"
#include <windows.h>
#include <d3d11.h>
#include <dxgi.h>
#include <atomic>
#include "imgui.h"
#include "imgui_impl_win32.h"
#include "imgui_impl_dx11.h"
#include "log.h"
#include "MinHook.h"

// Forward declarations
LRESULT __stdcall h_wndProc(const HWND hWnd, UINT uMsg, WPARAM wParam, LPARAM lParam);
bool FindAndHookD3D();
HRESULT __stdcall h_pPresent(IDXGISwapChain* pSwapChain, UINT SyncInterval, UINT Flags);
HRESULT __stdcall h_ResizeBuffers(IDXGISwapChain* pSwapChain, UINT BufferCount, UINT Width, UINT Height, DXGI_FORMAT NewFormat, UINT SwapChainFlags);

extern IMGUI_IMPL_API LRESULT ImGui_ImplWin32_WndProcHandler(HWND hWnd, UINT msg, WPARAM wParam, LPARAM lParam);

#pragma comment(lib, "d3d11.lib")
#pragma comment(lib, "dxgi.lib")

// --- GLOBAL VARIABLES ---
typedef HRESULT(__stdcall* D3D11Present_t)(IDXGISwapChain* pSwapChain, UINT SyncInterval, UINT Flags);
typedef HRESULT(__stdcall* D3D11ResizeBuffers_t)(IDXGISwapChain* pSwapChain, UINT BufferCount, UINT Width, UINT Height, DXGI_FORMAT NewFormat, UINT SwapChainFlags);

D3D11Present_t o_pPresent = nullptr;
D3D11ResizeBuffers_t o_pResizeBuffers = nullptr;

WNDPROC o_wndProc = nullptr;
HWND g_hGameWindow = NULL; // <<< ADDED THIS LINE to store the hooked window handle
std::atomic<bool> g_isHookInitialized = false;
std::atomic<bool> g_isImGuiInitialized = false;

ID3D11Device* g_pDevice = nullptr;
ID3D11DeviceContext* g_pContext = nullptr;
ID3D11RenderTargetView* g_pRenderTargetView = nullptr;

// --- HOOKED FUNCTIONS ---

LRESULT __stdcall h_wndProc(const HWND hWnd, UINT uMsg, WPARAM wParam, LPARAM lParam) {
    if (g_isImGuiInitialized && ImGui_ImplWin32_WndProcHandler(hWnd, uMsg, wParam, lParam)) return true;
    if (g_isImGuiInitialized) {
        ImGuiIO& io = ImGui::GetIO();
        if (io.WantCaptureMouse || io.WantCaptureKeyboard) {
            switch (uMsg) {
                case WM_LBUTTONDOWN: case WM_LBUTTONUP: case WM_RBUTTONDOWN: case WM_RBUTTONUP: case WM_MBUTTONDOWN:
                case WM_MBUTTONUP: case WM_MOUSEWHEEL: case WM_MOUSEMOVE: case WM_KEYDOWN: case WM_KEYUP: case WM_CHAR:
                    return 1;
            }
        }
    }
    return CallWindowProc(o_wndProc, hWnd, uMsg, wParam, lParam);
}

// --- MODIFIED ResizeBuffers Hook ---
HRESULT __stdcall h_ResizeBuffers(IDXGISwapChain* pSwapChain, UINT BufferCount, UINT Width, UINT Height, DXGI_FORMAT NewFormat, UINT SwapChainFlags) {
    Log("h_ResizeBuffers called. Renderer is resetting. Performing FULL cleanup.");

    if (g_pRenderTargetView) {
        g_pRenderTargetView->Release();
        g_pRenderTargetView = nullptr;
    }

    // --- CRITICAL FIX: Restore the original WndProc BEFORE destroying the ImGui context ---
    if (o_wndProc) {
        SetWindowLongPtr(g_hGameWindow, GWLP_WNDPROC, (LONG_PTR)o_wndProc);
        o_wndProc = nullptr; // Null out to signify it's unhooked.
        Log("Original WndProc restored to prevent crash.");
    }

    if (g_isImGuiInitialized) {
        ImGui_ImplDX11_Shutdown();
        ImGui_ImplWin32_Shutdown();
        ImGui::DestroyContext();
    }

    g_pDevice = nullptr;
    g_pContext = nullptr;
    g_isImGuiInitialized = false;

    Log("Cleanup complete. Calling original ResizeBuffers...");
    return o_pResizeBuffers(pSwapChain, BufferCount, Width, Height, NewFormat, SwapChainFlags);
}

// --- MODIFIED Present Hook ---
HRESULT __stdcall h_pPresent(IDXGISwapChain* pSwapChain, UINT SyncInterval, UINT Flags) {
    if (!g_isImGuiInitialized) {
        Log("h_pPresent: ImGui not initialized. Setting up...");
        if (SUCCEEDED(pSwapChain->GetDevice(__uuidof(ID3D11Device), (void**)&g_pDevice))) {
            g_pDevice->GetImmediateContext(&g_pContext);
            
            DXGI_SWAP_CHAIN_DESC desc;
            pSwapChain->GetDesc(&desc);
            g_hGameWindow = desc.OutputWindow; // Store the new/current window handle
            
            ImGui::CreateContext();
            ImGuiIO& io = ImGui::GetIO();
            io.ConfigFlags |= ImGuiConfigFlags_NavEnableKeyboard | ImGuiConfigFlags_NoMouseCursorChange;
            
            ImGui_ImplWin32_Init(g_hGameWindow);
            ImGui_ImplDX11_Init(g_pDevice, g_pContext);
            
            // Re-hook the WndProc if it was unhooked by ResizeBuffers
            if (o_wndProc == nullptr) {
                 o_wndProc = (WNDPROC)SetWindowLongPtr(g_hGameWindow, GWLP_WNDPROC, (LONG_PTR)h_wndProc);
                 Log("WndProc has been hooked.");
            }

            g_isImGuiInitialized = true;
            Log("SUCCESS: ImGui re-initialized.");
        }
    }

    if (g_pRenderTargetView == nullptr) {
        ID3D11Texture2D* pBackBuffer;
        // Check for success before using the back buffer
        if (SUCCEEDED(pSwapChain->GetBuffer(0, __uuidof(ID3D11Texture2D), (void**)&pBackBuffer))) {
            g_pDevice->CreateRenderTargetView(pBackBuffer, NULL, &g_pRenderTargetView);
            pBackBuffer->Release();
        }
    }

    ImGui_ImplDX11_NewFrame();
    ImGui_ImplWin32_NewFrame();
    ImGui::NewFrame();
    ImGui::ShowDemoWindow();

    ImGui::Render();
    if (g_pRenderTargetView) g_pContext->OMSetRenderTargets(1, &g_pRenderTargetView, NULL);
    ImGui_ImplDX11_RenderDrawData(ImGui::GetDrawData());

    return o_pPresent(pSwapChain, SyncInterval, Flags);
}

// --- SETUP FUNCTIONS (Unchanged from your file) ---

struct EnumData { DWORD dwProcessId; HWND hWnd; };
BOOL CALLBACK EnumProc(HWND hWnd, LPARAM lParam) {
    EnumData* pData = (EnumData*)lParam; DWORD dwProcessId = 0;
    GetWindowThreadProcessId(hWnd, &dwProcessId);
    if (dwProcessId == pData->dwProcessId && GetWindow(hWnd, GW_OWNER) == NULL && IsWindowVisible(hWnd)) {
        pData->hWnd = hWnd; return FALSE;
    } return TRUE;
}
HWND FindMainWindow(DWORD dwProcessId) {
    EnumData ed; ed.dwProcessId = dwProcessId; ed.hWnd = NULL;
    EnumWindows(EnumProc, (LPARAM)&ed); return ed.hWnd;
}

bool FindAndHookD3D() {
    if (g_isHookInitialized) return true;
    Log("--- Starting Reliable D3D Hook Setup ---");
    // No need to get the window here, h_pPresent will handle it dynamically
    
    WNDCLASSEX wc = { sizeof(WNDCLASSEX), CS_CLASSDC, DefWindowProc, 0, 0, GetModuleHandle(NULL), NULL, NULL, NULL, NULL, "DummyWindowClass", NULL };
    RegisterClassEx(&wc);
    HWND hDummyWnd = CreateWindow(wc.lpszClassName, NULL, WS_OVERLAPPEDWINDOW, 0, 0, 1, 1, NULL, NULL, wc.hInstance, NULL);
    DXGI_SWAP_CHAIN_DESC sd; ZeroMemory(&sd, sizeof(sd));
    sd.BufferCount = 1; sd.BufferDesc.Format = DXGI_FORMAT_R8G8B8A8_UNORM; sd.BufferUsage = DXGI_USAGE_RENDER_TARGET_OUTPUT;
    sd.OutputWindow = hDummyWnd; sd.SampleDesc.Count = 1; sd.Windowed = TRUE; sd.SwapEffect = DXGI_SWAP_EFFECT_DISCARD;
    IDXGISwapChain* pDummySwapChain; ID3D11Device* pDummyDevice;
    HRESULT hr = D3D11CreateDeviceAndSwapChain(NULL, D3D_DRIVER_TYPE_HARDWARE, NULL, 0, NULL, 0, D3D11_SDK_VERSION, &sd, &pDummySwapChain, &pDummyDevice, NULL, NULL);
    if (FAILED(hr)) { DestroyWindow(hDummyWnd); UnregisterClass(wc.lpszClassName, wc.hInstance); return false; }

    void** pVTable = *(void***)pDummySwapChain;
    void* pPresentAddress = pVTable[8];
    void* pResizeBuffersAddress = pVTable[13];
    pDummySwapChain->Release(); pDummyDevice->Release();
    DestroyWindow(hDummyWnd); UnregisterClass(wc.lpszClassName, wc.hInstance);

    if (MH_Initialize() != MH_OK) { return false; }
    if (MH_CreateHook(pPresentAddress, &h_pPresent, (void**)&o_pPresent) != MH_OK) { return false; }
    if (MH_CreateHook(pResizeBuffersAddress, &h_ResizeBuffers, (void**)&o_pResizeBuffers) != MH_OK) { return false; }
    if (MH_EnableHook(MH_ALL_HOOKS) != MH_OK) { return false; }
    Log("SUCCESS: All hooks are active!");
    g_isHookInitialized = true;
    return true;
}
