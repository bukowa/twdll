
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
HRESULT __stdcall h_pResizeBuffers(IDXGISwapChain* pSwapChain, UINT BufferCount, UINT Width, UINT Height, DXGI_FORMAT NewFormat, UINT SwapChainFlags);

extern IMGUI_IMPL_API LRESULT ImGui_ImplWin32_WndProcHandler(HWND hWnd, UINT msg, WPARAM wParam, LPARAM lParam);

#pragma comment(lib, "d3d11.lib")
#pragma comment(lib, "dxgi.lib")

typedef HRESULT(__stdcall* D3D11Present_t)(IDXGISwapChain* pSwapChain, UINT SyncInterval, UINT Flags);
typedef HRESULT(__stdcall* D3D11ResizeBuffers_t)(IDXGISwapChain* pSwapChain, UINT BufferCount, UINT Width, UINT Height, DXGI_FORMAT NewFormat, UINT SwapChainFlags);

D3D11Present_t o_pPresent = nullptr;
D3D11ResizeBuffers_t o_pResizeBuffers = nullptr;

WNDPROC o_wndProc = nullptr;
HWND g_hGameWindow = NULL;
std::atomic<bool> g_isHookInitialized = false;
std::atomic<bool> g_isImGuiInitialized = false;

ID3D11Device* g_pDevice = nullptr;
ID3D11DeviceContext* g_pContext = nullptr;
ID3D11RenderTargetView* g_pRenderTargetView = nullptr;


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
            
            static HWND lastHookedWindow = NULL;
            if (lastHookedWindow != g_hGameWindow) {
                if (lastHookedWindow != NULL && o_wndProc != NULL) {
                    SetWindowLongPtr(lastHookedWindow, GWLP_WNDPROC, (LONG_PTR)o_wndProc);
                    Log("Un-hooked WndProc from old window.");
                }
                o_wndProc = (WNDPROC)SetWindowLongPtr(g_hGameWindow, GWLP_WNDPROC, (LONG_PTR)h_wndProc);
                lastHookedWindow = g_hGameWindow;
                Log("WndProc has been hooked on new window.");
            }

            g_isImGuiInitialized = true;
            Log("SUCCESS: ImGui re-initialized.");
        }
    }

    if (g_pRenderTargetView) {
        g_pRenderTargetView->Release();
        g_pRenderTargetView = nullptr;
    }

    ID3D11Texture2D* pBackBuffer;
    if (SUCCEEDED(pSwapChain->GetBuffer(0, __uuidof(ID3D11Texture2D), (void**)&pBackBuffer))) {
        g_pDevice->CreateRenderTargetView(pBackBuffer, NULL, &g_pRenderTargetView);
        pBackBuffer->Release();
    } else {
        Log("ERROR: Failed to get back buffer from swap chain in h_pPresent.");
        return o_pPresent(pSwapChain, SyncInterval, Flags);
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

HRESULT __stdcall h_pResizeBuffers(IDXGISwapChain* pSwapChain, UINT BufferCount, UINT Width, UINT Height, DXGI_FORMAT NewFormat, UINT SwapChainFlags) {
    Log("--- h_pResizeBuffers called. Triggering full cleanup. ---");
    CleanupHooks();
    return o_pResizeBuffers(pSwapChain, BufferCount, Width, Height, NewFormat, SwapChainFlags);
}

bool FindAndHookD3D() {
    if (g_isHookInitialized) return true;
    Log("--- Starting Reliable D3D Hook Setup ---");
    
    WNDCLASSEX wc = { sizeof(WNDCLASSEX), CS_CLASSDC, DefWindowProc, 0, 0, GetModuleHandle(NULL), NULL, NULL, NULL, NULL, "DummyWindowClass", NULL };
    RegisterClassEx(&wc);
    HWND hDummyWnd = CreateWindow(wc.lpszClassName, NULL, WS_OVERLAPPEDWINDOW, 0, 0, 1, 1, NULL, NULL, wc.hInstance, NULL);
    DXGI_SWAP_CHAIN_DESC sd; ZeroMemory(&sd, sizeof(sd));
    sd.BufferCount = 1; sd.BufferDesc.Format = DXGI_FORMAT_R8G8B8A8_UNORM; sd.BufferUsage = DXGI_USAGE_RENDER_TARGET_OUTPUT;
    sd.OutputWindow = hDummyWnd; sd.SampleDesc.Count = 1; sd.Windowed = TRUE; sd.SwapEffect = DXGI_SWAP_EFFECT_DISCARD;
    IDXGISwapChain* pDummySwapChain; ID3D11Device* pDummyDevice;
    HRESULT hr = D3D11CreateDeviceAndSwapChain(NULL, D3D_DRIVER_TYPE_HARDWARE, NULL, 0, NULL, 0, D3D11_SDK_VERSION, &sd, &pDummySwapChain, &pDummyDevice, NULL, NULL);
    if (FAILED(hr)) { DestroyWindow(hDummyWnd); UnregisterClass(wc.lpszClassName, wc.hInstance); return false; }

    uintptr_t* pVTable = *reinterpret_cast<uintptr_t**>(pDummySwapChain);
    void* pPresentAddress = reinterpret_cast<void*>(pVTable[8]);
    void* pResizeBuffersAddress = reinterpret_cast<void*>(pVTable[13]);
    pDummySwapChain->Release(); pDummyDevice->Release();
    DestroyWindow(hDummyWnd); UnregisterClass(wc.lpszClassName, wc.hInstance);

    if (MH_Initialize() != MH_OK) { return false; }
    if (MH_CreateHook(pPresentAddress, &h_pPresent, (void**)&o_pPresent) != MH_OK) { return false; }
    if (MH_CreateHook(pResizeBuffersAddress, &h_pResizeBuffers, (void**)&o_pResizeBuffers) != MH_OK) { return false; }
    if (MH_EnableHook(MH_ALL_HOOKS) != MH_OK) { return false; }
    Log("SUCCESS: All hooks are active!");
    g_isHookInitialized = true;
    return true;
}

void CleanupHooks() {
    Log("--- Starting full cleanup... ---");

    // 1. Shutdown ImGui and release its resources
    if (g_isImGuiInitialized) {
        ImGui_ImplDX11_Shutdown();
        ImGui_ImplWin32_Shutdown();
        ImGui::DestroyContext();
        g_isImGuiInitialized = false;
        Log("ImGui has been shut down.");
    }

    // 2. Release our render target view
    if (g_pRenderTargetView) {
        g_pRenderTargetView->Release();
        g_pRenderTargetView = nullptr;
        Log("Render target view released.");
    }

    // 3. Restore the original window procedure
    if (g_hGameWindow && o_wndProc) {
        Log("Restoring original WndProc...");
        SetWindowLongPtr(g_hGameWindow, GWLP_WNDPROC, (LONG_PTR)o_wndProc);
        o_wndProc = nullptr; // Avoid trying to restore it again
    }

    // 4. Disable and uninitialize MinHook
    if (g_isHookInitialized) {
        Log("Disabling and uninitializing MinHook...");
        MH_DisableHook(MH_ALL_HOOKS);
        MH_Uninitialize();
        g_isHookInitialized = false;
    }
    
    Log("--- Full cleanup complete. ---");
}
