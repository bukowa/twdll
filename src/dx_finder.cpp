
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
static HWND g_lastHookedWindow = NULL; // Moved to global static scope

ID3D11Device* g_pDevice = nullptr;
ID3D11DeviceContext* g_pContext = nullptr;
ID3D11RenderTargetView* g_pRenderTargetView = nullptr;


LRESULT __stdcall h_wndProc(const HWND hWnd, UINT uMsg, WPARAM wParam, LPARAM lParam) {
    if (g_isImGuiInitialized) {
        // Let ImGui process the message first to update its internal state.
        // We will decide whether to consume the message based on io.WantCaptureMouse/Keyboard.
        ImGui_ImplWin32_WndProcHandler(hWnd, uMsg, wParam, lParam);

        ImGuiIO& io = ImGui::GetIO();
        // If ImGui wants to capture mouse or keyboard, and it's a relevant message, consume it.
        // This covers a broader range of mouse and keyboard messages.
        if ((io.WantCaptureMouse && (uMsg >= WM_MOUSEFIRST && uMsg <= WM_MOUSELAST)) ||
            (io.WantCaptureKeyboard && (uMsg >= WM_KEYFIRST && uMsg <= WM_KEYLAST))) {
            return 1; // Consume the message, preventing the game from receiving it.
        }
    }
    // If ImGui is not initialized, or it doesn't want to capture the input,
    // pass the message to the original window procedure.
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
            
            if (g_lastHookedWindow != g_hGameWindow) { // Use global static variable
                if (g_lastHookedWindow != NULL && o_wndProc != NULL) {
                    SetWindowLongPtr(g_lastHookedWindow, GWLP_WNDPROC, (LONG_PTR)o_wndProc);
                    Log("Un-hooked WndProc from old window.");
                }
                o_wndProc = (WNDPROC)SetWindowLongPtr(g_hGameWindow, GWLP_WNDPROC, (LONG_PTR)h_wndProc);
                g_lastHookedWindow = g_hGameWindow; // Update global static variable
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
    Log("FindAndHookD3D: Entry. g_isHookInitialized before: %d", g_isHookInitialized.load());
    if (g_isHookInitialized) {
        Log("FindAndHookD3D: Hooks already initialized, returning.");
        return true;
    }
    Log("--- Starting Reliable D3D Hook Setup ---");
    
    WNDCLASSEX wc = { sizeof(WNDCLASSEX), CS_CLASSDC, DefWindowProc, 0, 0, GetModuleHandle(NULL), NULL, NULL, NULL, NULL, "DummyWindowClass", NULL };
    RegisterClassEx(&wc);
    HWND hDummyWnd = CreateWindow(wc.lpszClassName, NULL, WS_OVERLAPPEDWINDOW, 0, 0, 1, 1, NULL, NULL, wc.hInstance, NULL);
    DXGI_SWAP_CHAIN_DESC sd; ZeroMemory(&sd, sizeof(sd));
    sd.BufferCount = 1; sd.BufferDesc.Format = DXGI_FORMAT_R8G8B8A8_UNORM; sd.BufferUsage = DXGI_USAGE_RENDER_TARGET_OUTPUT;
    sd.OutputWindow = hDummyWnd; sd.SampleDesc.Count = 1; sd.Windowed = TRUE; sd.SwapEffect = DXGI_SWAP_EFFECT_DISCARD;
    IDXGISwapChain* pDummySwapChain; ID3D11Device* pDummyDevice;
    HRESULT hr = D3D11CreateDeviceAndSwapChain(NULL, D3D_DRIVER_TYPE_HARDWARE, NULL, 0, NULL, 0, D3D11_SDK_VERSION, &sd, &pDummySwapChain, &pDummyDevice, NULL, NULL);
    if (FAILED(hr)) { Log("FindAndHookD3D: D3D11CreateDeviceAndSwapChain FAILED. Error: %lx", hr); DestroyWindow(hDummyWnd); UnregisterClass(wc.lpszClassName, wc.hInstance); return false; }

    uintptr_t* pVTable = *reinterpret_cast<uintptr_t**>(pDummySwapChain);
    void* pPresentAddress = reinterpret_cast<void*>(pVTable[8]);
    void* pResizeBuffersAddress = reinterpret_cast<void*>(pVTable[13]);
    pDummySwapChain->Release(); pDummyDevice->Release();
    DestroyWindow(hDummyWnd); UnregisterClass(wc.lpszClassName, wc.hInstance); 

    if (MH_Initialize() != MH_OK) { Log("FindAndHookD3D: MH_Initialize FAILED."); return false; }
    if (MH_CreateHook(pPresentAddress, &h_pPresent, (void**)&o_pPresent) != MH_OK) { Log("FindAndHookD3D: MH_CreateHook pPresentAddress FAILED."); return false; }
    if (MH_CreateHook(pResizeBuffersAddress, &h_pResizeBuffers, (void**)&o_pResizeBuffers) != MH_OK) { Log("FindAndHookD3D: MH_CreateHook pResizeBuffersAddress FAILED."); return false; }
    if (MH_EnableHook(MH_ALL_HOOKS) != MH_OK) { Log("FindAndHookD3D: MH_EnableHook FAILED."); return false; }
    Log("SUCCESS: All hooks are active!");
    g_isHookInitialized = true;
    Log("FindAndHookD3D: Exit. g_isHookInitialized after: %d", g_isHookInitialized.load());
    return true;
}

void CleanupHooks() {
    Log("CleanupHooks: Entry. g_isHookInitialized before: %d", g_isHookInitialized.load());
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
        g_lastHookedWindow = NULL; // Correctly reset the global static variable
    }

    // 4. Disable and uninitialize MinHook
    if (g_isHookInitialized) {
        Log("Disabling and uninitializing MinHook...");
        MH_DisableHook(MH_ALL_HOOKS);
        MH_Uninitialize();
        g_isHookInitialized = false;
    }
    
    Log("--- Full cleanup complete. ---");
    Log("CleanupHooks: Exit. g_isHookInitialized after: %d", g_isHookInitialized.load());
}
