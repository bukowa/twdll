#!/bin/bash

# --- Configuration ---
# The name of the game's process
PROCESS_NAME="Rome2.exe"

# The path to the injector executable
INJECTOR_PATH="./InjectorCLIx86.exe"

# The path to the ScyllaHide DLL to inject
# Make sure this points to the correct 32-bit (x86) or 64-bit (x64) DLL
DLL_PATH="./HookLibraryx86.dll"
# --- End of Configuration ---

echo "--- ScyllaHide Auto-Injector ---"

# Check if the required DLL exists
if [ ! -f "$DLL_PATH" ]; then
    echo "Error: The DLL was not found at the specified path: $DLL_PATH"
    exit 1
fi

# Find the Process ID (PID) of the game
# We use tasklist, filter for the process name (case-insensitive with -i),
# and then use awk to grab the second column, which is the PID.
echo "Searching for process: $PROCESS_NAME"
PID=$(tasklist | grep -a -i "$PROCESS_NAME" | head -n 1 | awk '{print $2}')


# Check if the PID was found. If the variable is empty, the process isn't running.
if [ -z "$PID" ]; then
    echo "Error: Process '$PROCESS_NAME' not found."
    echo "Please make sure the game is running (and suspended in IDA) before running this script."
    exit 1
fi

# If we get here, the PID was found successfully
echo "Process found! PID: $PID"
echo "Injecting $DLL_PATH..."

# Execute the injector with the found PID and DLL path
echo "$INJECTOR_PATH" pid:"$PID" "$DLL_PATH"
"$INJECTOR_PATH" pid:"$PID" "$DLL_PATH"

echo "------------------------------------"
echo "Injection command executed. Check for success/error messages above."
echo "You can now resume the process in IDA (F9)."
