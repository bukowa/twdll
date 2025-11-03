#pragma once
#include <dxgi.h>
#include <functional>
#include <windows.h>

// Forward declarations for DirectX types to reduce header dependencies.
struct IDXGISwapChain;
struct ID3D11Device;
struct ID3D11DeviceContext;
struct ID3D11RenderTargetView;

/**
 * @class ImGuiD3D11Hook
 * @brief Manages hooking of a D3D11 application for rendering with ImGui.
 *
 * This class encapsulates all the logic for finding the D3D11 Present function,
 * hooking it using MinHook, and setting up an ImGui context for rendering.
 * It follows the Singleton pattern because the C-style hook callbacks need a single,
 * well-defined object to interact with.
 *
 * RAII is leveraged: the destructor ensures that all hooks are removed and
 * resources are cleaned up properly.
 *
 * Usage:
 * 1. Call ImGuiD3D11Hook::Install(renderCallback); to install the hooks.
 *    The renderCallback is a function that contains your ImGui rendering calls.
 * 2. Call ImGuiD3D11Hook::Uninstall(); to remove the hooks and clean up.
 */
class ImGuiD3D11Hook {
public:
    // Type alias for the user-provided rendering function.
    using RenderCallback = std::function<void()>;

    /**
     * @brief Installs the D3D11 hooks and initializes ImGui.
     * @param onRender The callback function that will be executed each frame to render the UI.
     * @return True if installation was successful, false otherwise.
     */
    static bool Install(RenderCallback onRender);

    /**
     * @brief Uninstalls the hooks, cleans up ImGui, and releases all resources.
     */
    static void Uninstall();

    // Deleted copy and move operations to enforce singleton behavior.
    ImGuiD3D11Hook(const ImGuiD3D11Hook&) = delete;
    ImGuiD3D11Hook& operator=(const ImGuiD3D11Hook&) = delete;

private:
    // Private constructor and destructor to be managed by Install/Uninstall.
    ImGuiD3D11Hook(RenderCallback onRender);
    ~ImGuiD3D11Hook();

    // --- Singleton Instance ---
    static ImGuiD3D11Hook* s_instance;
    static ImGuiD3D11Hook& GetInstance();

    // --- Core Hooking Logic ---
    bool Hook();
    void Unhook();

    // --- ImGui and Graphics Management ---
    void InitializeImGui(IDXGISwapChain* pSwapChain);
    void ShutdownImGui();
    void CreateRenderTarget(IDXGISwapChain* pSwapChain);
    void ReleaseRenderTarget();

    // --- Window Procedure Management ---
    void HookWindow(HWND hWnd);
    void UnhookWindow();

    // --- Hooked Functions (Static) ---
    // These must be static as they are C-style callbacks. They will use GetInstance()
    // to access the class members.
    static LRESULT __stdcall Hook_WndProc(HWND hWnd, UINT uMsg, WPARAM wParam, LPARAM lParam);
    static HRESULT __stdcall Hook_Present(IDXGISwapChain* pSwapChain, UINT SyncInterval, UINT Flags);
    static HRESULT __stdcall Hook_ResizeBuffers(IDXGISwapChain* pSwapChain, UINT BufferCount, UINT Width, UINT Height, DXGI_FORMAT NewFormat, UINT SwapChainFlags);
    
    // --- Typedefs for Original Functions ---
    using D3D11Present_t = HRESULT(__stdcall*)(IDXGISwapChain*, UINT, UINT);
    using D3D11ResizeBuffers_t = HRESULT(__stdcall*)(IDXGISwapChain*, UINT, UINT, UINT, DXGI_FORMAT, UINT);

    // --- Member Variables ---
    RenderCallback m_onRender;

    // DirectX state
    ID3D11Device*           m_pDevice = nullptr;
    ID3D11DeviceContext*    m_pContext = nullptr;
    ID3D11RenderTargetView* m_pRenderTargetView = nullptr;

    // Window state
    HWND    m_hGameWindow = NULL;
    WNDPROC m_oWndProc = nullptr;
    
    // Hooking state
    D3D11Present_t       m_oPresent = nullptr;
    D3D11ResizeBuffers_t m_oResizeBuffers = nullptr;

    // Status flags
    bool m_isImGuiInitialized = false;
    bool m_hooksInstalled = false;
};