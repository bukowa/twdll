import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import ghidra.app.script.GhidraScript;
import ghidra.app.util.template.TemplateSimplifier;
import ghidra.program.model.address.AddressRange;
import ghidra.program.model.address.AddressSetView;
import ghidra.program.model.data.StringDataInstance;
import ghidra.program.model.listing.CodeUnitFormat;
import ghidra.program.model.listing.CodeUnitFormatOptions;
import ghidra.program.model.listing.Data;
import ghidra.program.model.listing.Function;
import ghidra.program.model.listing.FunctionIterator;
import ghidra.program.model.listing.Instruction;
import ghidra.program.model.listing.InstructionIterator;
import ghidra.program.model.listing.Listing;
import ghidra.program.util.DefinedDataIterator;

import java.io.BufferedWriter;
import java.io.FileOutputStream;
import java.io.OutputStreamWriter;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Locale;
import java.util.Set;

public class TwdllSilverExport extends GhidraScript {
    private static final Gson GSON = new GsonBuilder().disableHtmlEscaping().create();

    @SuppressWarnings("unused")
    private static class SilverFuncRec {
        final String ea;
        final long size;
        final List<String[]> ranges;
        final String name;
        final String signature;
        final String calling_conv;
        final int stack_purge;
        final boolean is_thunk;
        final boolean is_noret;
        final String thunk_target_ea;

        SilverFuncRec(
                String ea,
                long size,
                List<String[]> ranges,
                String name,
                String signature,
                String callingConv,
                int stackPurge,
                boolean isThunk,
                boolean isNoret,
                String thunkTargetEa
        ) {
            this.ea = ea;
            this.size = size;
            this.ranges = ranges;
            this.name = name;
            this.signature = signature;
            this.calling_conv = callingConv;
            this.stack_purge = stackPurge;
            this.is_thunk = isThunk;
            this.is_noret = isNoret;
            this.thunk_target_ea = thunkTargetEa;
        }
    }

    @SuppressWarnings("unused")
    private static class SilverAsmRec {
        final String ea;
        final int insn_count;
        final String raw_asm;

        SilverAsmRec(String ea, int insnCount, String rawAsm) {
            this.ea = ea;
            this.insn_count = insnCount;
            this.raw_asm = rawAsm;
        }
    }

    @SuppressWarnings("unused")
    private static class SilverStringRec {
        final String ea;
        final String encoding;
        final int length;
        final String value;
        final int ref_count;

        SilverStringRec(String ea, String encoding, int length, String value, int refCount) {
            this.ea = ea;
            this.encoding = encoding;
            this.length = length;
            this.value = value;
            this.ref_count = refCount;
        }
    }

    private static String normalizeStringEncoding(String charset, String dataTypeName) {
        String c = charset != null ? charset.toLowerCase(Locale.ROOT) : "";
        String dt = dataTypeName != null ? dataTypeName.toLowerCase(Locale.ROOT) : "";
        if (c.contains("utf-16") || c.contains("utf16") || dt.contains("unicode16") || (dt.contains("unicode") && !dt.contains("32"))) {
            return "utf16le";
        }
        if (c.contains("utf-32") || c.contains("utf32") || dt.contains("unicode32")) {
            return "utf32le";
        }
        if (c.contains("utf-8") || c.contains("utf8") || dt.contains("utf8")) {
            return "utf8";
        }
        return "ascii";
    }

    private static String hexEA(long v) {
        return "0x" + Long.toHexString(v);
    }

    @Override
    public void run() throws Exception {
        String outPath = System.getenv("TWDLL_SILVER_OUT");
        if (outPath == null || outPath.isEmpty()) {
            throw new IllegalArgumentException("TWDLL_SILVER_OUT is required");
        }
        String asmOutPath = System.getenv("TWDLL_SILVER_ASM_OUT");
        boolean exportAsm = asmOutPath != null && !asmOutPath.isEmpty();
        String stringsOutPath = System.getenv("TWDLL_SILVER_STRINGS_OUT");
        boolean exportStrings = stringsOutPath != null && !stringsOutPath.isEmpty();

        if (currentProgram == null) {
            throw new IllegalStateException("currentProgram is null");
        }

        long t0 = System.currentTimeMillis();
        int count = 0;

        String totalOutPath = System.getenv("TWDLL_SILVER_TOTAL_OUT");
        if (totalOutPath != null && !totalOutPath.isEmpty()) {
            int totalHint = currentProgram.getFunctionManager().getFunctionCount();
            Files.writeString(Path.of(totalOutPath), Integer.toString(totalHint), StandardCharsets.UTF_8);
        }

        CodeUnitFormat cuf = null;
        Listing listing = null;
        if (exportAsm) {
            cuf = new CodeUnitFormat(new CodeUnitFormatOptions(
                    CodeUnitFormatOptions.ShowBlockName.NEVER,
                    CodeUnitFormatOptions.ShowNamespace.NON_LOCAL,
                    null,
                    true,
                    true,
                    true,
                    true,
                    false, // Browser default: avoids bogus +0x4 adjustments on EBP-relative stack refs.
                    true,
                    true,
                    new TemplateSimplifier()
            ));
            listing = currentProgram.getListing();
        }

        BufferedWriter asmBw = null;
        try (BufferedWriter bw = new BufferedWriter(
                new OutputStreamWriter(new FileOutputStream(outPath), StandardCharsets.UTF_8), 65536)) {
            if (exportAsm) {
                asmBw = new BufferedWriter(
                        new OutputStreamWriter(new FileOutputStream(asmOutPath), StandardCharsets.UTF_8), 65536);
            }

            FunctionIterator it = currentProgram.getFunctionManager().getFunctions(true);
            StringBuilder asmBuf = exportAsm ? new StringBuilder(1024) : null;
            while (it.hasNext() && !monitor.isCancelled()) {
                Function fn = it.next();
                if (fn == null || fn.isExternal()) {
                    continue;
                }

                long ea = fn.getEntryPoint().getOffset();
                String eaHex = hexEA(ea);
                AddressSetView body = fn.getBody();
                long size = body != null ? body.getNumAddresses() : 0;

                List<String[]> ranges = new ArrayList<>();
                if (body != null) {
                    for (AddressRange r : body.getAddressRanges()) {
                        if (!r.getMinAddress().isMemoryAddress()) continue;
                        long rStart = r.getMinAddress().getOffset();
                        long rEnd = rStart + r.getLength();
                        ranges.add(new String[]{hexEA(rStart), hexEA(rEnd)});
                    }
                }

                String name = fn.getName(true);
                if (name == null) name = "";
                String signature = fn.getPrototypeString(false, true);
                if (signature == null) signature = "";
                String cc = fn.getCallingConventionName();
                if (cc == null) cc = "";
                int purge = fn.isStackPurgeSizeValid() ? fn.getStackPurgeSize() : -1;
                boolean isThunk = fn.isThunk();
                boolean isNoRet = fn.hasNoReturn();
                long thunkTarget = 0;
                if (isThunk) {
                    Function tgt = fn.getThunkedFunction(false);
                    if (tgt != null && !tgt.isExternal()) {
                        thunkTarget = tgt.getEntryPoint().getOffset();
                    }
                }

                bw.write(GSON.toJson(new SilverFuncRec(
                        eaHex,
                        size,
                        ranges,
                        name,
                        signature,
                        cc,
                        purge,
                        isThunk,
                        isNoRet,
                        hexEA(thunkTarget)
                )));
                bw.newLine();

                if (asmBw != null) {
                    asmBuf.setLength(0);
                    int insnCount = 0;
                    if (body != null) {
                        InstructionIterator insIt = listing.getInstructions(body, true);
                        while (insIt.hasNext() && !monitor.isCancelled()) {
                            Instruction ins = insIt.next();
                            if (insnCount > 0) {
                                asmBuf.append('\n');
                            }
                            asmBuf.append(Long.toHexString(ins.getAddress().getOffset()))
                                    .append(": ")
                                    .append(cuf.getRepresentationString(ins, true));
                            insnCount++;
                        }
                    }
                    asmBw.write(GSON.toJson(new SilverAsmRec(eaHex, insnCount, asmBuf.toString())));
                    asmBw.newLine();
                }

                count++;
                if (count == 1 || (count <= 50 && count % 10 == 0) || count % 500 == 0) {
                    bw.flush();
                }
            }
            bw.flush();
            if (asmBw != null) {
                asmBw.flush();
            }
        } finally {
            if (asmBw != null) {
                asmBw.close();
            }
        }
        println(String.format("TwdllSilverExport: exported %d functions in %.2fs", count, (System.currentTimeMillis() - t0) / 1000.0));

        if (exportStrings && !monitor.isCancelled()) {
            long ts0 = System.currentTimeMillis();
            int strCount = 0;
            Set<Long> seenStringEas = new HashSet<>();
            try (BufferedWriter sbw = new BufferedWriter(
                    new OutputStreamWriter(new FileOutputStream(stringsOutPath), StandardCharsets.UTF_8), 65536)) {
                for (Data d : DefinedDataIterator.byDataInstance(currentProgram, StringDataInstance::isString)) {
                    if (monitor.isCancelled()) {
                        break;
                    }
                    if (d == null || !StringDataInstance.isString(d)) {
                        continue;
                    }
                    StringDataInstance sdi = StringDataInstance.getStringDataInstance(d);
                    if (sdi == null) {
                        continue;
                    }
                    String val = sdi.getStringValue();
                    if (val == null || val.isEmpty()) {
                        continue;
                    }
                    long sEa = d.getMinAddress().getOffset();
                    if (!seenStringEas.add(sEa)) {
                        continue;
                    }
                    int len = sdi.getStringLength();
                    if (len <= 0) {
                        len = val.length();
                    }
                    String enc = normalizeStringEncoding(sdi.getCharsetName(), d.getDataType().getName());
                    int refs = currentProgram.getReferenceManager().getReferenceCountTo(d.getMinAddress());
                    sbw.write(GSON.toJson(new SilverStringRec(hexEA(sEa), enc, len, val, refs)));
                    sbw.newLine();
                    strCount++;
                }
                sbw.flush();
            }
            println(String.format("TwdllSilverExport: exported %d strings in %.2fs", strCount, (System.currentTimeMillis() - ts0) / 1000.0));
        }
    }
}
