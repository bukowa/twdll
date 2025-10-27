import pyperclip
import re
import sys

def get_hex_input(prompt_text, default_value=None):
    """ A function to get a hex address from input, understands 'address - name' format. """
    base_addr_pattern = re.compile(r"^\s*([0-9A-Fa-f]{8,})")
    while True:
        try:
            prompt = f"{prompt_text}"
            if default_value:
                prompt += f" (default: {default_value}): "
            else:
                prompt += ": "
            user_input = input(prompt).strip()
            if not user_input and default_value:
                return int(default_value, 16)
            match = base_addr_pattern.match(user_input)
            if match:
                address_part = match.group(1)
                return int(address_part, 16)
            else:
                return int(user_input, 16)
        except (ValueError, TypeError):
            print("Error! Please enter a valid hexadecimal address.")

def rebase_address(initial_base_address_str=None, initial_ida_base_address_str=None):
    """
    The main script function. Can accept initial process base and IDA base as arguments.
    Reads multiline text until an empty line is encountered.
    """
    print("\n--- ULTIMATE PRO Address Rebaser for IDA ---")
    live_base_addr = None
    ida_base_addr = None

    # --- NEW: Process the second argument for IDA base ---
    if initial_ida_base_address_str:
        try:
            ida_base_addr = int(initial_ida_base_address_str, 16)
            print(f"Info: Received starting IDA base address: {hex(ida_base_addr).upper().replace('0X', '')}")
        except (ValueError, TypeError):
            print("Warning: Could not parse the provided IDA base address argument.")

    if initial_base_address_str:
        base_addr_pattern = re.compile(r"^\s*([0-9A-Fa-f]{8,})")
        match = base_addr_pattern.match(initial_base_address_str.strip())
        if match:
            try:
                live_base_addr = int(match.group(1), 16)
                print(f"Info: Received starting process base address: {hex(live_base_addr).upper().replace('0X', '')}")
            except ValueError: pass

    # Fallback to interactive input if arguments were not provided or invalid
    if live_base_addr is None:
        try: live_base_addr = get_hex_input("Enter the module's base address (e.g., 'address - name.dll')")
        except KeyboardInterrupt: print("\nInterrupted."); return

    if ida_base_addr is None:
        try:
            ida_base_addr = get_hex_input("Enter the module's base address in IDA Pro", "10000000")
        except KeyboardInterrupt: print("\nInterrupted."); return

    print("-" * 40)
    print(f"✅ Process Base: {hex(live_base_addr).upper().replace('0X', '')}")
    print(f"✅ IDA Pro Base: {hex(ida_base_addr).upper().replace('0X', '')}")
    print("-" * 40)
    print("Ready! Paste text and finish with an empty line (double Enter).")

    base_addr_pattern = re.compile(r"^\s*([0-9A-Fa-f]{8,})\s*-.*$")

    while True:
        try:
            print("\n> Waiting for data paste...")

            lines = []
            while True:
                line = sys.stdin.readline()
                if not line.strip():
                    break
                lines.append(line)
            pasted_text = "".join(lines)

            if not pasted_text.strip():
                continue

            match = base_addr_pattern.match(pasted_text.strip())
            if match:
                new_base_addr_str = match.group(1)
                live_base_addr = int(new_base_addr_str, 16)
                print(f"\n🔄 NEW PROCESS BASE SET TO: {hex(live_base_addr).upper().replace('0X', '')}")
                continue

            found_address_str = None
            match = re.search(r"^([0-9A-Fa-f]{8,})\s-.*<<", pasted_text, re.MULTILINE)
            if match: found_address_str = match.group(1)

            if not found_address_str:
                match = re.search(r"^([0-9A-Fa-f]{8,})\s-", pasted_text, re.MULTILINE)
                if match: found_address_str = match.group(1)

            if not found_address_str:
                match = re.search(r"^\s*([0-9A-Fa-f]{8,})\s*$", pasted_text.strip())
                if match: found_address_str = match.group(1)

            if found_address_str:
                live_instruction_addr = int(found_address_str, 16)
                offset = live_instruction_addr - live_base_addr
                ida_address = ida_base_addr + offset
                ida_address_hex = hex(ida_address).upper().replace("0X", "")

                print("--- Rebased! ---")
                print(f"  Live Address:         {hex(live_instruction_addr).upper().replace('0X', '')}")
                print(f"  Offset:               {hex(offset).upper().replace('0X', '')}")
                print(f"  Address for IDA Pro:  {ida_address_hex}")

                pyperclip.copy(ida_address_hex)
                print("\n✅ Address for IDA has been copied to the clipboard!")
            else:
                print("❌ No address to rebase was found in the provided text.")

        except KeyboardInterrupt:
            print("\nScript finished. Goodbye!")
            break
        except Exception as e:
            print(f"\nAn error occurred: {e}")

if __name__ == "__main__":
    # Example of calling with both arguments provided
    rebase_address("68240000 - Rome2.dll", "10000000")