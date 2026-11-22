#include "signature_scanner.h"
#include <cstring>

namespace Scanner {

// Simple helper to check if a byte matches a pattern byte (considering wildcards)
static bool byte_matches(const unsigned char byte, const unsigned char pattern_byte, const unsigned char mask_byte) {
    return (byte & mask_byte) == (pattern_byte & mask_byte);
}

// Signature scanning function
uintptr_t find_signature(uintptr_t base_address, size_t size, const char* signature_str) {
    // Convert signature string to byte array and generate mask
    std::vector<unsigned char> signature_bytes;
    std::vector<unsigned char> mask_bytes;

    std::stringstream ss_sig(signature_str);
    std::string byte_hex;
    while (ss_sig >> byte_hex) {
        if (byte_hex == "??" || byte_hex == "?") {
            signature_bytes.push_back(0x00); // Wildcard, actual value doesn't matter
            mask_bytes.push_back(0x00);     // Mask out this byte
        } else {
            signature_bytes.push_back(static_cast<unsigned char>(std::stoul(byte_hex, nullptr, 16)));
            mask_bytes.push_back(0xFF);     // Match this byte exactly
        }
    }

    const size_t sig_len = signature_bytes.size();
    if (sig_len == 0 || sig_len >= size) return 0;
    const size_t last = size - sig_len;  // candidates are i in [0, last)

    // Anchor on the first non-wildcard byte and jump between its occurrences with
    // memchr instead of testing every offset. Candidates are still visited in
    // increasing order, so the result (first match) is identical to a linear scan.
    size_t anchor = 0;
    while (anchor < sig_len && mask_bytes[anchor] == 0) ++anchor;
    if (anchor == sig_len) return base_address;  // all wildcards

    const unsigned char* mem = reinterpret_cast<const unsigned char*>(base_address);
    const unsigned char anchor_byte = signature_bytes[anchor];
    size_t pos = anchor;                  // position of the anchor byte in memory
    const size_t pos_end = last + anchor; // exclusive
    while (pos < pos_end) {
        const void* hit = memchr(mem + pos, anchor_byte, pos_end - pos);
        if (!hit) break;
        pos = static_cast<size_t>(static_cast<const unsigned char*>(hit) - mem);
        const size_t i = pos - anchor;
        bool found = true;
        for (size_t j = 0; j < sig_len; ++j) {
            if (!byte_matches(mem[i + j], signature_bytes[j], mask_bytes[j])) {
                found = false;
                break;
            }
        }
        if (found) {
            return base_address + i;
        }
        ++pos;
    }
    return 0; // Not found
}

uintptr_t FindString(uintptr_t base, size_t size, const char* needle) {
    size_t nlen = strlen(needle);
    if (nlen == 0 || nlen + 1 > size) return 0;
    const char* mem = reinterpret_cast<const char*>(base);
    const size_t end = size - nlen;  // last candidate, inclusive
    size_t i = 0;
    while (i <= end) {
        const void* hit = memchr(mem + i, needle[0], end - i + 1);
        if (!hit) break;
        i = static_cast<size_t>(static_cast<const char*>(hit) - mem);
        if (memcmp(mem + i, needle, nlen) == 0 && mem[i + nlen] == '\0')
            return base + i;
        ++i;
    }
    return 0;
}

uintptr_t FindPushRef(uintptr_t text_base, size_t text_size, uintptr_t target_addr) {
    const uint8_t* mem = reinterpret_cast<const uint8_t*>(text_base);
    if (text_size < 5) return 0;
    const size_t end = text_size - 5;  // last candidate, inclusive
    size_t i = 0;
    while (i <= end) {
        const void* hit = memchr(mem + i, 0x68, end - i + 1);
        if (!hit) break;
        i = static_cast<size_t>(static_cast<const uint8_t*>(hit) - mem);
        uintptr_t imm = *reinterpret_cast<const uintptr_t*>(mem + i + 1);
        if (imm == target_addr)
            return text_base + i;
        ++i;
    }
    return 0;
}

static bool is_prologue(const uint8_t* p) {
    // push ebp; mov ebp, esp (55 8B EC)
    if (p[0] == 0x55 && p[1] == 0x8B && p[2] == 0xEC)
        return true;
    // push ebp; mov ebp, ecx (55 8B ED) - common thiscall optimization
    if (p[0] == 0x55 && p[1] == 0x8B && p[2] == 0xED)
        return true;
    // sub esp, imm32 (81 EC XX XX XX XX)
    if (p[0] == 0x81 && p[1] == 0xEC)
        return true;
    // sub esp, imm8 (83 EC XX)
    if (p[0] == 0x83 && p[1] == 0xEC)
        return true;
    // push ebx; push ebp; push esi; push edi
    if (p[0] == 0x53 && p[1] == 0x55 && p[2] == 0x56 && p[3] == 0x57)
        return true;
    // push ebx; push esi; push edi; mov edi, ecx
    if (p[0] == 0x53 && p[1] == 0x56 && p[2] == 0x57 && p[3] == 0x8B && p[4] == 0xF9)
        return true;
    return false;
}

uintptr_t FindPrologue(uintptr_t from_addr, size_t max_back) {
    uintptr_t start = (from_addr > max_back) ? from_addr - max_back : 0;
    
    // Walk backwards from the instruction before the push
    for (uintptr_t addr = from_addr - 1; addr >= start; --addr) {
        const uint8_t* p = reinterpret_cast<const uint8_t*>(addr);
        if (is_prologue(p)) {
            // Check if preceded by padding (CC or 90)
            uint8_t prev = *reinterpret_cast<const uint8_t*>(addr - 1);
            if (prev == 0xCC || prev == 0x90) {
                return addr;
            }
        }
    }
    return 0;
}

} // namespace Scanner
