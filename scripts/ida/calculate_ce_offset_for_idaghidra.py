import pyperclip
import re
import sys


def get_hex_input(prompt_text, default_value=None):
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
                return int(match.group(1), 16)
            return int(user_input, 16)
        except (ValueError, TypeError):
            print("Error! Please enter a valid hexadecimal address.")


def rebase_address(initial_base_address_str=None, initial_ida_base_address_str=None):
    print("\n--- AUTO-CALCULATING Address Rebaser for IDA ---")
    live_base_addr = None
    ida_base_addr = None

    if initial_ida_base_address_str:
        try:
            ida_base_addr = int(initial_ida_base_address_str, 16)
        except:
            pass

    if ida_base_addr is None:
        ida_base_addr = get_hex_input("Enter your IDA Image Base (check Edit -> Segments -> Rebase program)",
                                      "10000000")

    print(f"✅ IDA Pro Base: {hex(ida_base_addr).upper().replace('0X', '')}")
    print("Ready! Paste CE Disassembly (including the 'Module.dll+Offset:' line).")
    print("Example: Empire.Retail.dll+13977E8:")

    while True:
        try:
            print("\n> Waiting for data paste (Double Enter to process)...")
            lines = []
            while True:
                line = sys.stdin.readline()
                if not line.strip(): break
                lines.append(line)
            pasted_text = "".join(lines)
            if not pasted_text.strip(): continue

            # 1. Look for the Header: Module.dll+Offset:
            # Matches: Empire.Retail.dll+13977E8:
            header_match = re.search(r"([\w\.]+)\+([0-9A-Fa-f]+):", pasted_text)

            # 2. Look for the Instruction Address (the one with << or just the first address)
            # Matches: 61F777E8 - F3 0F10...
            addr_match = re.search(r"([0-9A-Fa-f]{8,})\s-.*<<", pasted_text)
            if not addr_match:
                addr_match = re.search(r"([0-9A-Fa-f]{8,})\s-", pasted_text)

            if header_match and addr_match:
                module_name = header_match.group(1)
                ce_offset = int(header_match.group(2), 16)
                live_instr_addr = int(addr_match.group(1), 16)

                # Auto-calculate the live base of the DLL
                live_base_addr = live_instr_addr - ce_offset

                # Calculate IDA Address
                ida_address = ida_base_addr + ce_offset
                ida_address_hex = hex(ida_address).upper().replace("0X", "")

                print("-" * 40)
                print(f"📦 Module:      {module_name}")
                print(f"📍 Live Base:    {hex(live_base_addr).upper().replace('0X', '')}")
                print(f"🔢 Offset:       {hex(ce_offset).upper().replace('0X', '')}")
                print(f"🚀 IDA Address:  {ida_address_hex}")
                print("-" * 40)

                pyperclip.copy(ida_address_hex)
                print("✅ Address copied to clipboard!")

            elif addr_match and live_base_addr:
                # Fallback if no header but we already know a base address
                live_instr_addr = int(addr_match.group(1), 16)
                offset = live_instr_addr - live_base_addr
                ida_address = ida_base_addr + offset
                print(f"Rebased via current base: {hex(ida_address).upper()}")
                pyperclip.copy(hex(ida_address).upper().replace("0X", ""))
            else:
                print("❌ Could not find Module+Offset or Address. Make sure to include the top line!")

        except KeyboardInterrupt:
            break
        except Exception as e:
            print(f"Error: {e}")


if __name__ == "__main__":
    rebase_address(None, "10000000")
