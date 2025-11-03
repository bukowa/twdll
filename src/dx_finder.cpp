#include "dx_finder.h"
#include <d3d11.h>
#include <dxgi.h>

// Link necessary libraries
#pragma comment(lib, "d3d11.lib")
#pragma comment(lib, "dxgi.lib")

// Include necessary headers
#include "MinHook.h"
#include "imgui.h"
#include "imgui_impl_win32.h"
#include "imgui_impl_dx11.h"
#include "log.h" // Assuming spdlog is encapsulated or available here

// Forward declare the ImGui Win32 handler
extern IMGUI_IMPL_API LRESULT ImGui_ImplWin32_WndProcHandler(HWND hWnd, UINT msg, WPARAM wParam, LPARAM lParam);

// V-table indices for the functions we want to hook
namespace VTable {
constexpr int Present = 8;
constexpr int ResizeBuffers = 13;
} // namespace VTable

// --- Singleton Implementation ---
ImGuiD3D11Hook *ImGuiD3D11Hook::s_instance = nullptr;

bool ImGuiD3D11Hook::Install(RenderCallback onRender) {
    if (s_instance) {
        spdlog::warn("ImGuiD3D11Hook is already installed.");
        return true;
    }
    try {
        s_instance = new ImGuiD3D11Hook(std::move(onRender));
        if (s_instance->Hook()) {
            return true;
        }
        // If hook fails, clean up the instance.
        delete s_instance;
        s_instance = nullptr;
        return false;
    } catch (const std::exception &e) {
        spdlog::error("Failed to install ImGuiD3D11Hook: {}", e.what());
        if (s_instance) {
            delete s_instance;
            s_instance = nullptr;
        }
        return false;
    }
}

void ImGuiD3D11Hook::Uninstall() {
    if (!s_instance) {
        spdlog::warn("ImGuiD3D11Hook is not installed, nothing to uninstall.");
        return;
    }
    delete s_instance;
    s_instance = nullptr;
}

ImGuiD3D11Hook &ImGuiD3D11Hook::GetInstance() {
    // This should only be called from the static hooks when s_instance is valid.
    return *s_instance;
}

// --- Constructor / Destructor ---
ImGuiD3D11Hook::ImGuiD3D11Hook(RenderCallback onRender)
    : m_onRender(std::move(onRender)) {
    spdlog::info("ImGuiD3D11Hook instance created.");
}

// RAII: Destructor ensures everything is cleaned up.
ImGuiD3D11Hook::~ImGuiD3D11Hook() {
    Unhook();
    spdlog::info("ImGuiD3D11Hook instance destroyed.");
}

// --- Core Hooking Logic ---
bool ImGuiD3D11Hook::Hook() {
    spdlog::info("--- Starting D3D11 Hook Setup ---");

    // 1. Create a dummy device and swap chain to get the V-Table addresses
    WNDCLASSEX wc = {sizeof(WNDCLASSEX), CS_CLASSDC, DefWindowProc, 0L, 0L, GetModuleHandle(NULL), NULL, NULL, NULL, NULL, "Dummy", NULL};
    RegisterClassEx(&wc);
    HWND hDummyWnd = CreateWindow(wc.lpszClassName, NULL, WS_OVERLAPPEDWINDOW, 100, 100, 1, 1, NULL, NULL, wc.hInstance, NULL);

    DXGI_SWAP_CHAIN_DESC sd;
    ZeroMemory(&sd, sizeof(sd));
    sd.BufferCount = 1;
    sd.BufferDesc.Format = DXGI_FORMAT_R8G8B8A8_UNORM;
    sd.BufferUsage = DXGI_USAGE_RENDER_TARGET_OUTPUT;
    sd.OutputWindow = hDummyWnd;
    sd.SampleDesc.Count = 1;
    sd.Windowed = TRUE;
    sd.SwapEffect = DXGI_SWAP_EFFECT_DISCARD;

    IDXGISwapChain *pDummySwapChain;
    ID3D11Device *pDummyDevice;
    HRESULT hr = D3D11CreateDeviceAndSwapChain(NULL, D3D_DRIVER_TYPE_HARDWARE, NULL, 0, NULL, 0, D3D11_SDK_VERSION, &sd, &pDummySwapChain, &pDummyDevice, NULL, NULL);

    if (FAILED(hr)) {
        spdlog::error("D3D11CreateDeviceAndSwapChain failed. Error: {:#x}", static_cast<unsigned int>(hr));
        DestroyWindow(hDummyWnd);
        UnregisterClass(wc.lpszClassName, wc.hInstance);
        return false;
    }

    // 2. Extract V-Table function pointers
    uintptr_t *pVTable = *reinterpret_cast<uintptr_t **>(pDummySwapChain);
    void *pPresentAddress = reinterpret_cast<void *>(pVTable[VTable::Present]);
    void *pResizeBuffersAddress = reinterpret_cast<void *>(pVTable[VTable::ResizeBuffers]);

    // 3. Clean up dummy resources
    pDummySwapChain->Release();
    pDummyDevice->Release();
    DestroyWindow(hDummyWnd);
    UnregisterClass(wc.lpszClassName, wc.hInstance);

    // 4. Initialize and apply hooks with MinHook
    if (MH_Initialize() != MH_OK) {
        spdlog::error("MinHook initialization failed.");
        return false;
    }
    if (MH_CreateHook(pPresentAddress, &Hook_Present, reinterpret_cast<void **>(&m_oPresent)) != MH_OK) {
        spdlog::error("Failed to create hook for IDXGISwapChain::Present.");
        MH_Uninitialize();
        return false;
    }
    if (MH_CreateHook(pResizeBuffersAddress, &Hook_ResizeBuffers, reinterpret_cast<void **>(&m_oResizeBuffers)) != MH_OK) {
        spdlog::error("Failed to create hook for IDXGISwapChain::ResizeBuffers.");
        MH_RemoveHook(pPresentAddress);
        MH_Uninitialize();
        return false;
    }
    if (MH_EnableHook(MH_ALL_HOOKS) != MH_OK) {
        spdlog::error("Failed to enable hooks.");
        MH_RemoveHook(pPresentAddress);
        MH_RemoveHook(pResizeBuffersAddress);
        MH_Uninitialize();
        return false;
    }

    m_hooksInstalled = true;
    spdlog::info("SUCCESS: D3D11 hooks installed and enabled.");
    return true;
}

void ImGuiD3D11Hook::Unhook() {
    if (!m_hooksInstalled)
        return;

    spdlog::info("--- Starting D3D11 Hook Cleanup ---");

    ShutdownImGui(); // This also unhooks WndProc

    MH_DisableHook(MH_ALL_HOOKS);
    MH_Uninitialize();

    m_hooksInstalled = false;
    spdlog::info("SUCCESS: D3D11 hooks have been removed.");
}

// --- ImGui & Graphics Management ---
void ImGuiD3D11Hook::InitializeImGui(IDXGISwapChain *pSwapChain) {
    if (m_isImGuiInitialized)
        return;

    spdlog::info("Initializing ImGui...");

    if (FAILED(pSwapChain->GetDevice(__uuidof(ID3D11Device), reinterpret_cast<void **>(&m_pDevice)))) {
        spdlog::error("Failed to get D3D11Device from swap chain.");
        return;
    }
    m_pDevice->GetImmediateContext(&m_pContext);

    DXGI_SWAP_CHAIN_DESC desc;
    pSwapChain->GetDesc(&desc);
    m_hGameWindow = desc.OutputWindow;

    HookWindow(m_hGameWindow);

    ImGui::CreateContext();
    ImGuiIO &io = ImGui::GetIO();
    io.ConfigFlags |= ImGuiConfigFlags_NavEnableKeyboard;

    // This tells ImGui to not change the mouse cursor at all.
    io.ConfigFlags |= ImGuiConfigFlags_NoMouseCursorChange;

    ImGui_ImplWin32_Init(m_hGameWindow);
    ImGui_ImplDX11_Init(m_pDevice, m_pContext);

    m_isImGuiInitialized = true;
    spdlog::info("SUCCESS: ImGui initialized for window {}.", (void *)m_hGameWindow);
}

void ImGuiD3D11Hook::ShutdownImGui() {
    if (!m_isImGuiInitialized)
        return;

    spdlog::info("Shutting down ImGui...");

    UnhookWindow();

    ImGui_ImplDX11_Shutdown();
    ImGui_ImplWin32_Shutdown();
    ImGui::DestroyContext();

    ReleaseRenderTarget();

    if (m_pContext) {
        m_pContext->Release();
        m_pContext = nullptr;
    }
    if (m_pDevice) {
        m_pDevice->Release();
        m_pDevice = nullptr;
    }

    m_isImGuiInitialized = false;
    spdlog::info("ImGui has been shut down.");
}

void ImGuiD3D11Hook::CreateRenderTarget(IDXGISwapChain *pSwapChain) {
    ReleaseRenderTarget(); // Ensure no existing RTV is leaked

    ID3D11Texture2D *pBackBuffer;
    if (SUCCEEDED(pSwapChain->GetBuffer(0, __uuidof(ID3D11Texture2D), (void **)&pBackBuffer))) {
        m_pDevice->CreateRenderTargetView(pBackBuffer, NULL, &m_pRenderTargetView);
        pBackBuffer->Release();
    } else {
        spdlog::error("Failed to get back buffer from swap chain.");
    }
}

void ImGuiD3D11Hook::ReleaseRenderTarget() {
    if (m_pRenderTargetView) {
        m_pRenderTargetView->Release();
        m_pRenderTargetView = nullptr;
    }
}

// --- Window Procedure Management ---
void ImGuiD3D11Hook::HookWindow(HWND hWnd) {
    if (m_oWndProc)
        return; // Already hooked

    m_oWndProc = (WNDPROC)SetWindowLongPtr(hWnd, GWLP_WNDPROC, (LONG_PTR)Hook_WndProc);
    spdlog::info("WndProc hooked.");
}

void ImGuiD3D11Hook::UnhookWindow() {
    if (!m_oWndProc)
        return; // Not hooked

    SetWindowLongPtr(m_hGameWindow, GWLP_WNDPROC, (LONG_PTR)m_oWndProc);
    m_oWndProc = nullptr;
    spdlog::info("WndProc unhooked.");
}

// --- Static Hook Handlers ---

HRESULT __stdcall ImGuiD3D11Hook::Hook_Present(IDXGISwapChain *pSwapChain, UINT SyncInterval, UINT Flags) {
    auto &instance = GetInstance();

    // One-time initialization of ImGui
    if (!instance.m_isImGuiInitialized) {
        instance.InitializeImGui(pSwapChain);
    }

    // Recreate render target if it's missing (e.g., after a resize)
    if (!instance.m_pRenderTargetView) {
        instance.CreateRenderTarget(pSwapChain);
    }

    // Force the cursor to be visible every frame. This overrides any
    // attempts by the game to hide it.
    ShowCursor(TRUE);

    // Begin new ImGui frame
    ImGui_ImplDX11_NewFrame();
    ImGui_ImplWin32_NewFrame();
    ImGui::NewFrame();

    // Execute the user's rendering code
    if (instance.m_onRender) {
        instance.m_onRender();
    } else { // Fallback for safety
        ImGui::ShowDemoWindow();
    }

    // Render ImGui draw data
    ImGui::Render();
    if (instance.m_pRenderTargetView) {
        instance.m_pContext->OMSetRenderTargets(1, &instance.m_pRenderTargetView, NULL);
        ImGui_ImplDX11_RenderDrawData(ImGui::GetDrawData());
    }

    return instance.m_oPresent(pSwapChain, SyncInterval, Flags);
}

HRESULT __stdcall ImGuiD3D11Hook::Hook_ResizeBuffers(IDXGISwapChain *pSwapChain, UINT BufferCount, UINT Width, UINT Height, DXGI_FORMAT NewFormat, UINT SwapChainFlags) {
    auto &instance = GetInstance();

    spdlog::info("ResizeBuffers called. Releasing render target.");

    // This is the correct, minimal way to handle resize.
    // We only release the render target view. The Present hook will recreate it.
    instance.ReleaseRenderTarget();

    // We also need to inform the ImGui DX11 backend so it can release its own resources.
    // The backend will be re-initialized on the next Present call automatically.
    // For simplicity and robustness, a full re-init is often safer.
    // However, the ImplDX11 has an InvalidateDeviceObjects function which is ideal.
    // If not available, shutting down and letting Present re-init is an option.
    // Here, we just release our RTV. The ImGui backend handles its state internally.
    ImGui_ImplDX11_InvalidateDeviceObjects();

    // Call the original function to let the application handle the resize.
    HRESULT result = instance.m_oResizeBuffers(pSwapChain, BufferCount, Width, Height, NewFormat, SwapChainFlags);

    // ImGui's DX11 backend will create new device objects on the next call to NewFrame
    // ImGui_ImplDX11_CreateDeviceObjects(); -> This is called internally by NewFrame if needed.

    return result;
}

LRESULT __stdcall ImGuiD3D11Hook::Hook_WndProc(HWND hWnd, UINT uMsg, WPARAM wParam, LPARAM lParam) {
    auto &instance = GetInstance();

    if (instance.m_isImGuiInitialized) {
        // Let ImGui process the message. It will update its internal state (e.g., mouse position).
        ImGui_ImplWin32_WndProcHandler(hWnd, uMsg, wParam, lParam);

        // Check if ImGui wants to capture input
        ImGuiIO &io = ImGui::GetIO();

        // Only block messages from the game if ImGui is actively using them.
        // This prevents us from blocking essential window messages that are not direct input.
        if (io.WantCaptureMouse && (uMsg >= WM_MOUSEFIRST && uMsg <= WM_MOUSELAST)) {
            return 1; // ImGui is using the mouse, so we eat the message.
        }
        if (io.WantCaptureKeyboard && (uMsg >= WM_KEYFIRST && uMsg <= WM_KEYLAST)) {
            return 1; // ImGui is using the keyboard, so we eat the message.
        }
    }

    // If ImGui doesn't want the input, or it's not an input message,
    // pass it along to the original game window procedure.
    return CallWindowProc(instance.m_oWndProc, hWnd, uMsg, wParam, lParam);
}