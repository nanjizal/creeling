package creeling;

import creeling.FlowContext;
import creeling.ASTNode;
import creeling.HxbNodeKind;
import creeling.Lifetime;

class CreelingTyper {
    public var context = new FlowContext();
    public var specializedClones = new Map< String, Array<String>>();
    public function new(){}
    public function specializeAndCheck( nodes: Array<ASTNode> ): Array<String> {
        var outputInstructions = new Array<String>();
        var len = nodes.length;
        var node: ASTNode;
        for( i in 0...len ){
            node = nodes[i];
            switch( node.kind ){
                case NodeAlloc: 
                    context.variables.set( node.varID
                        , {   id: node.varID
                            , type: ( node.typeName != null )? node.typeName: "Unknown"
                            , state: Owned } );
                    outputInstructions.push( "ALLOC_OBJ var_" + node.varID + "<" + node.typeName +">" );
                case NodeVarAccess:
                    var track = context.variables.get( node.varID );
                    if( track != null && track.state == Owned ){
                        outputInstructions.push( "READ_OWNED var_ " + node.varID );
                    } else { 
                        outputInstructions.push( "READ_BORROW var_ " + node.varID );
                    }
                case NodeCall: 
                    var track = context.variables.get( node.varID );
                    var argState = ( track != null && track.state == Owned )? "Owned": "Borrow";
                    var variantSignature = node.methodID + "_Varient_" + argState;
                    if( !specializedClones.exists(node.methodID) ){
                        specializedClones.set( node.methodID, [] );
                    }
                    specializedClones.get( node.methodID ).push( variantSignature );
                    outputInstructions.push( "CALL_METHOD " + variantSignature + " via var_" + node.varID );
                case NodeFieldStore:
                    var track = context.variables.get( node.varID );
                    if( track != null ){
                        track.state = Leaked;
                        outputInstructions.push("UPGRADE_TO_RUNTIME_RC var_" + node.varID );
                    }
                case NodeBlock:
                    if( node.children != null ){
                        outputInstructions = outputInstructions.concat(specializeAndCheck(node.children));
                    }
                case NodeIfElse:
                    var thenSnapshot = context.clone();
                    var elseSnapshot = context.clone();
                    context = thenSnapshot;
                    var thenInstructions = ( node.thenBlock != null )? specializeAndCheck( node.thenBlock ): [];
                    context = elseSnapshot;
                    var elseInstructions = ( node.elseBlock != null )? specializeAndCheck( node.elseBlock ): [];
                    context = mergeBranchUnification( thenSnapshot, elseSnapshot );
                    outputInstructions.push( "START_BRANCH_IF");
                    outputInstructions = outputInstructions.concat( thenInstructions );
                    outputInstructions.push( "BRANCH_ELSE" );
                    outputInstructions = outputInstructions.concat( elseInstructions );
                    outputInstructions.push("END_BRANCH");
            }
            evaluateLivenessPruning( node.varID, i, nodes, outputInstructions);
        }
        return outputInstructions;
    }
    private function mergeBranchUnification( thenCtx: FlowContext, elseCtx: FlowContext ): FlowContext {
        var merged = new FlowContext();
        for( id in thenCtx.variables.keys() ){
            var thenTrack = thenCtx.variables.get( id );
            var elseTrack = elseCtx.variables.get( id );
            if( elseTrack == null ) continue;
            if( thenTrack.state == Leaked || elseTrack.state == Leaked ) {
                merged.variables.set( id, { id: id, type: thenTrack.type, state: Leaked } );
            } else if ( thenTrack.state == Owned && elseTrack.state == Owned ){
                merged.variables.set( id, { id: id, type: thenTrack.type, state: Owned } );
            } else {
                merged.variables.set( id, { id: id, type: thenTrack.type, state: Leaked } );
            }
        }
        return merged;
    }
    private function evaluateLivenessPruning(  varID: Int
                                             , currentIdx: Int
                                             , totalStream: Array<ASTNode>
                                             , instructions: Array<String> ): Void {
        var track = context.variables.get( varID );
        if (track == null || track.state == null || track.state != Owned) return;
        var isUsedLater = false;
        // currentIdx check needed here??
        for( i in ( currentIdx + 1 )...totalStream.length ){
            if( totalStream[ i ].varID  == varID ){
                isUsedLater = true;
                break;
            }
        }
        if( !isUsedLater ){
            instructions.push( " LFR3_FREE var_" + varID );
            context.variables.remove( varID );
        }
    }
}
