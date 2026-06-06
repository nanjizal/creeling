package creeling;
import creeling.*;
class Test {
    public static function main(){
        var program: Array<ASTNode> = [ { kind: NodeAlloc, varID: 101, typeName: "MathVector" }
                                       , { kind: NodeVarAccess, varID: 101 }
                                       , { kind: NodeIfElse, varID: 0
                                          , thenBlock: [ { kind: NodeCall, varId: 101, method: "calculateLength" }]
                                          , elseBlock: [ { kind: NodeFieldStore, varID: 101 }]
                                         }];
        var typer = new CreelingTyper();
        var compiledInstructions = typer.specalizeAndCheck( program );
        trace( "Generated Annotate Bytecode Stream:" );
        for( inst in compiledInstructions ) trace( " - " + inst );
        trace( "\nGenerated Specialied Function Map:");
        for( origin in typer.specalizedClones.keys() ) trace( " - Method: " + origin + " cloned variant: " + typer.specializedClones.get( origin ));
    }
}

