# A plugin that registers a hotkey (Shift+W) to close pseudocode windows.
#
# HOW TO INSTALL AND USE:
# 1. Save this file as "close_pseudocode_plugin.py".
# 2. Find your IDA plugins directory.
# 3. Copy the "close_pseudocode_plugin.py" and "ida_plugin.json" file into that directory.
# 4. Restart IDA Pro. The hotkey will now be active.
# 5. Press Shift+W anytime to run the script.

import ida_kernwin
import ida_idaapi
import string

# --- This is our working logic from the previous script ---
# We use ida_kernwin.msg() instead of print() for better IDA integration.
def close_other_pseudocode_views():
    """
    Closes all pseudocode windows except "Pseudocode-A" by finding them
    by their expected names.
    """
    window_title_to_keep = "Pseudocode-A"
    ida_kernwin.msg(f"--- Hotkey (Shift+W) Activated ---\n")

    widgets_to_close = []

    # Loop through "Pseudocode-B", "Pseudocode-C", etc.
    for char in string.ascii_uppercase[1:]: # Start from 'B'
        title_to_find = f"Pseudocode-{char}"
        
        # Use the find_widget() function that we know works for you.
        widget = ida_kernwin.find_widget(title_to_find)

        if widget:
            widget_type = ida_kernwin.get_widget_type(widget)
            if widget_type == ida_kernwin.BWN_PSEUDOCODE:
                ida_kernwin.msg(f"Found window to close: '{title_to_find}'\n")
                widgets_to_close.append(widget)

    if not widgets_to_close:
        ida_kernwin.msg("No other pseudocode windows (B-Z) were found.\n")
    else:
        count = len(widgets_to_close)
        for widget_to_close in widgets_to_close:
            title = ida_kernwin.get_widget_title(widget_to_close)
            ida_kernwin.msg(f"Closing window: '{title}'\n")
            ida_kernwin.close_widget(widget_to_close, 0)
        
        ida_kernwin.msg(f"Successfully closed {count} pseudocode window(s).\n")

    ida_kernwin.msg("--- Script finished ---\n")
    return


# --- This is the standard IDA plugin structure ---
class PseudocodeCloserPlugin(ida_idaapi.plugin_t):
    # Mandatory plugin properties
    flags = ida_idaapi.PLUGIN_KEEP
    comment = "Adds a hotkey to close all but one pseudocode window"
    help = "Press Shift+W to close other pseudocode windows"
    wanted_name = "Pseudocode Closer Hotkey"
    wanted_hotkey = ""  # The hotkey is registered manually below

    def init(self):
        """This is called when the plugin is loaded."""
        # Define the hotkey. You can change "Shift-W" to anything you want.
        self.hotkey_shortcut = "Shift-W"
        
        # Register the hotkey and store its context handle.
        # This uses the add_hotkey function we confirmed you have.
        self.hotkey_context = ida_kernwin.add_hotkey(
            self.hotkey_shortcut,
            close_other_pseudocode_views)

        if self.hotkey_context is None:
            ida_kernwin.msg(f"Error: Failed to register hotkey '{self.hotkey_shortcut}'. It might be in use.\n")
            return ida_idaapi.PLUGIN_SKIP
        
        ida_kernwin.msg(f"Plugin 'Pseudocode Closer' loaded. Hotkey '{self.hotkey_shortcut}' is ready.\n")
        return ida_idaapi.PLUGIN_OK

    def run(self, arg):
        """This is called when the plugin is run from the menu. Not needed for a hotkey."""
        pass

    def term(self):
        """This is called when the plugin is unloaded (e.g., IDA is closing)."""
        # Clean up by removing the hotkey.
        if hasattr(self, 'hotkey_context'):
            ida_kernwin.del_hotkey(self.hotkey_context)
        ida_kernwin.msg("Plugin 'Pseudocode Closer' unloaded.\n")


# This is the required function that tells IDA how to load the plugin.
def PLUGIN_ENTRY():
    return PseudocodeCloserPlugin()