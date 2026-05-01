
import xml.etree.ElementTree as ET
import idaapi
import idc
import struct

def get_module_base(mod_name):
    """
    Finds the base address of a loaded module by name.
    """
    mod = idaapi.modinfo_t()
    if idaapi.get_first_module(mod):
        while True:
            if mod.name.lower().endswith(mod_name.lower()):
                return mod.base
            if not idaapi.get_next_module(mod):
                break
    return None

def read_ptr(address):
    """
    Reads a 32-bit pointer from a given memory address.
    """
    ptr_bytes = idaapi.dbg_read_memory(address, 4)
    if ptr_bytes:
        return struct.unpack('<I', ptr_bytes)[0]
    return None

def set_breakpoint_from_xml(xml_data):
    """
    Parses a Cheat Engine XML, follows the pointer chain, and sets a hardware R/W breakpoint.
    """
    try:
        root = ET.fromstring(xml_data)
    except ET.ParseError as e:
        print(f"Error parsing XML: {e}")
        return

    cheat_entry = root.find('.//CheatEntry')
    if cheat_entry is None:
        print("Could not find a CheatEntry in the XML.")
        return

    address_str = cheat_entry.find('Address').text
    offsets_nodes = cheat_entry.findall('Offsets/Offset')

    try:
        parts = address_str.strip('"').split('"+')
        mod_name = parts[0]
        base_offset = int(parts[1], 16)
    except (ValueError, IndexError):
        print(f"Could not parse address string: {address_str}")
        return

    offsets = [int(offset.text, 16) for offset in offsets_nodes]
    offsets.reverse() # Offsets are from bottom to top

    print(f"Module: {mod_name}, Base Offset: {hex(base_offset)}, Offsets: {[hex(o) for o in offsets]}")

    if not idaapi.is_debugger_on():
        print("Error: Debugger is not active.")
        return

    module_base = get_module_base(mod_name)
    if not module_base:
        print(f"Module '{mod_name}' not found.")
        return

    current_address = module_base + base_offset
    print(f"Base address of {mod_name}: {hex(module_base)}")
    print(f"Starting address: {hex(current_address)}")

    # Follow the pointer chain
    for i, offset in enumerate(offsets):
        print(f"Reading pointer at: {hex(current_address)}")
        ptr = read_ptr(current_address)
        if ptr is None:
            print(f"Failed to read pointer at {hex(current_address)}")
            return
        current_address = ptr + offset
        print(f"Offset {i} ({hex(offset)}): New address is {hex(current_address)}")

    final_address = current_address
    print(f"Final address: {hex(final_address)}")

    # Set a hardware read/write breakpoint.
    size = 4 # Assuming a 4-byte value
    if idc.add_bpt(final_address, size, idaapi.BPT_RDWR):
        print(f"Successfully set a R/W hardware breakpoint at {hex(final_address)}")
    else:
        print(f"Failed to set a breakpoint at {hex(final_address)}")

# XML data from Cheat Engine
xml_data = """<?xml version="1.0" encoding="utf-8"?>
<CheatTable>
  <CheatEntries>
    <CheatEntry>
      <ID>397</ID>
      <Description>"battle this pointer 0"</Description>
      <LastState Value="CCCCCCC3" RealAddress="69DC87FA"/>
      <ShowAsHex>1</ShowAsHex>
      <ShowAsSigned>0</ShowAsSigned>
      <VariableType>4 Bytes</VariableType>
      <Address>"Rome2.dll"+01FD53F8</Address>
      <Offsets>
        <Offset>0</Offset>
        <Offset>4</Offset>
        <Offset>20</Offset>
        <Offset>8</Offset>
      </Offsets>
    </CheatEntry>
  </CheatEntries>
</CheatTable>
"""

# To use this script in IDA Pro:
# 1. Attach the IDA debugger to the target process.
# 2. Suspend the process.
# 3. Run this script from File -> Script file...
set_breakpoint_from_xml(xml_data.strip())
