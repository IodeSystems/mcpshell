# Test plan: st_1m7k4fc4v4r3e6ryj9107n

Remove all 29 dead `goto errorExit` lines from the ANTLR-generated parser, and remove any `errorExit:` labels that become unused as a result.

```sh inputs=parser/mcpshell_parser.go
python3 - << 'PYEOF'
import re

with open('parser/mcpshell_parser.go', 'r') as f:
    content = f.read()

# Split into functions by finding function boundaries
# Each rule function ends with the dead goto pattern
# We need to process each function separately

# Strategy: find all occurrences of the dead goto, and for each one,
# check if there are other "goto errorExit" in the same function.
# A function starts with "func (p *McpShellParser)" and ends with "}"

lines = content.split('\n')
result = []
i = 0
removed_gotos = 0
removed_labels = 0

while i < len(lines):
    line = lines[i]
    
    # Check if this is the dead goto line
    if line.strip() == 'goto errorExit // Trick to prevent compiler error if the label is not used':
        # Look backwards to find the start of this function
        # and check for other "goto errorExit" references
        func_start = None
        for j in range(i-1, -1, -1):
            if lines[j].startswith('func (p *McpShellParser)'):
                func_start = j
                break
        
        if func_start is not None:
            # Count "goto errorExit" in this function (excluding the current dead one)
            other_gotos = 0
            for j in range(func_start, i):
                if 'goto errorExit' in lines[j] and 'Trick' not in lines[j]:
                    other_gotos += 1
            
            if other_gotos == 0:
                # No other goto references this label - also remove the label
                # Find the "errorExit:" label line
                for j in range(i-1, func_start, -1):
                    if lines[j].strip() == 'errorExit:':
                        # Remove the label by marking it
                        lines[j] = '__REMOVE_LABEL__'
                        removed_labels += 1
                        break
            
            # Skip the dead goto line (don't add it to result)
            removed_gotos += 1
            i += 1
            continue
        else:
            # Shouldn't happen, but keep the line
            result.append(line)
            i += 1
            continue
    else:
        # Skip marked label lines
        if line == '__REMOVE_LABEL__':
            i += 1
            continue
        result.append(line)
        i += 1

with open('parser/mcpshell_parser.go', 'w') as f:
    f.write('\n'.join(result))

print(f"Removed {removed_gotos} dead goto lines, {removed_labels} unused labels")
PYEOF
```

Remove the 5 remaining unused `errorExit:` label lines at the specific line numbers reported by the build.

```sh inputs=parser/mcpshell_parser.go
python3 - << 'PYEOF'
with open('parser/mcpshell_parser.go', 'r') as f:
    lines = f.readlines()

# The 5 lines (1-indexed) where "label errorExit defined and not used"
target_lines = [5706, 8135, 8278, 8402, 18042]

# Remove those lines (convert to 0-indexed)
to_remove = set(l - 1 for l in target_lines)
result = [line for i, line in enumerate(lines) if i not in to_remove]

with open('parser/mcpshell_parser.go', 'w') as f:
    f.writelines(result)

print(f"Removed {len(to_remove)} unused label lines")
PYEOF
```
